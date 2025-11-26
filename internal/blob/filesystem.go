package blob

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
)

// Ensure FileSystemStore implements the Store interface at compile time
var _ Store = (*FileSystemStore)(nil)

// FileSystemStore provides file system-based blob storage.
// It implements the Store interface for local development and testing.
// Note: FileSystemStore does not implement PresignedURLGenerator as
// pre-signed URLs are not applicable to local file systems.
type FileSystemStore struct {
	basePath string
	logger   *logger.Logger
	mu       sync.RWMutex // Protects concurrent file operations
}

// FileSystemOption defines functional options for configuring FileSystemStore
type FileSystemOption func(*fileSystemOptions)

type fileSystemOptions struct {
	createBasePath bool
	permissions    os.FileMode
}

// defaultFileSystemOptions returns sensible defaults
func defaultFileSystemOptions() *fileSystemOptions {
	return &fileSystemOptions{
		createBasePath: true,
		permissions:    0755,
	}
}

// WithCreateBasePath controls whether to create the base path if it doesn't exist
func WithCreateBasePath(create bool) FileSystemOption {
	return func(o *fileSystemOptions) {
		o.createBasePath = create
	}
}

// WithPermissions sets the file permissions for created directories
func WithPermissions(perm os.FileMode) FileSystemOption {
	return func(o *fileSystemOptions) {
		o.permissions = perm
	}
}

// NewFileSystemStore creates a new file system-based blob store.
// The basePath specifies the root directory for storing blobs.
func NewFileSystemStore(basePath string, log *logger.Logger, opts ...FileSystemOption) (*FileSystemStore, error) {
	if basePath == "" {
		return nil, fmt.Errorf("base path is required")
	}

	options := defaultFileSystemOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}

	// Create base directory if it doesn't exist
	if options.createBasePath {
		if err := os.MkdirAll(absPath, options.permissions); err != nil {
			return nil, fmt.Errorf("failed to create base path: %w", err)
		}
	} else {
		// Verify the path exists
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("base path does not exist: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("base path is not a directory")
		}
	}

	log.Info("file system blob store initialized", "basePath", absPath)

	return &FileSystemStore{
		basePath: absPath,
		logger:   log,
	}, nil
}

// fullPath constructs the full file path for a key
func (f *FileSystemStore) fullPath(key string) (string, error) {
	if key == "" {
		return "", domain.ErrInvalidBlobKey
	}

	// Prevent path traversal attacks
	cleanKey := filepath.Clean(key)
	if strings.HasPrefix(cleanKey, "..") || filepath.IsAbs(cleanKey) {
		return "", fmt.Errorf("%w: invalid key path", domain.ErrInvalidBlobKey)
	}

	return filepath.Join(f.basePath, cleanKey), nil
}

// Upload uploads an object to the file system.
func (f *FileSystemStore) Upload(ctx context.Context, input *UploadInput) (*UploadOutput, error) {
	if input.Key == "" {
		return nil, domain.ErrInvalidBlobKey
	}

	if input.Body == nil {
		return nil, fmt.Errorf("%w: body is required", domain.ErrInvalidInput)
	}

	fullPath, err := f.fullPath(input.Key)
	if err != nil {
		return nil, err
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		f.logger.Error("failed to create directory",
			"key", input.Key,
			"path", dir,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}

	// Check context before starting write
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Create a temporary file first to ensure atomic writes
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		f.logger.Error("failed to create temp file",
			"key", input.Key,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // Clean up temp file on error
	}()

	// Calculate MD5 hash while writing
	hash := md5.New()
	writer := io.MultiWriter(tmpFile, hash)

	written, err := io.Copy(writer, input.Body)
	if err != nil {
		f.logger.Error("failed to write file",
			"key", input.Key,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}

	// Close the temp file before renaming
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, fullPath); err != nil {
		f.logger.Error("failed to rename temp file",
			"key", input.Key,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}

	etag := hex.EncodeToString(hash.Sum(nil))

	f.logger.Debug("file uploaded successfully",
		"key", input.Key,
		"bytes", written,
	)

	return &UploadOutput{
		Location: fullPath,
		ETag:     etag,
	}, nil
}

// Download downloads an object into the provided writer.
func (f *FileSystemStore) Download(ctx context.Context, key string, w io.WriterAt) (int64, error) {
	fullPath, err := f.fullPath(key)
	if err != nil {
		return 0, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	file, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, domain.ErrBlobNotFound
		}
		f.logger.Error("failed to open file",
			"key", key,
			"error", err,
		)
		return 0, fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}
	defer file.Close()

	// Check context before reading
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	// Read file and write to WriterAt at offset 0
	data, err := io.ReadAll(file)
	if err != nil {
		f.logger.Error("failed to read file",
			"key", key,
			"error", err,
		)
		return 0, fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}

	n, err := w.WriteAt(data, 0)
	if err != nil {
		return int64(n), fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}

	f.logger.Debug("file downloaded successfully",
		"key", key,
		"bytes", n,
	)

	return int64(n), nil
}

// GetObject retrieves an object and returns it as a ReadCloser.
// The caller is responsible for closing the returned reader.
func (f *FileSystemStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath, err := f.fullPath(key)
	if err != nil {
		return nil, err
	}

	f.mu.RLock()
	file, err := os.Open(fullPath)
	f.mu.RUnlock()

	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, domain.ErrBlobNotFound
		}
		f.logger.Error("failed to open file",
			"key", key,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}

	return file, nil
}

// HeadObject retrieves metadata about an object without reading its contents.
func (f *FileSystemStore) HeadObject(ctx context.Context, key string) (*ObjectInfo, error) {
	fullPath, err := f.fullPath(key)
	if err != nil {
		return nil, err
	}

	f.mu.RLock()
	info, err := os.Stat(fullPath)
	f.mu.RUnlock()

	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, domain.ErrBlobNotFound
		}
		f.logger.Error("failed to stat file",
			"key", key,
			"error", err,
		)
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	if info.IsDir() {
		return nil, domain.ErrBlobNotFound
	}

	return &ObjectInfo{
		Key:          key,
		Size:         info.Size(),
		ContentType:  detectContentType(key),
		LastModified: info.ModTime(),
	}, nil
}

// Delete removes an object from the file system.
func (f *FileSystemStore) Delete(ctx context.Context, key string) error {
	fullPath, err := f.fullPath(key)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if err := os.Remove(fullPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Consider delete of non-existent file as success (idempotent)
			return nil
		}
		f.logger.Error("failed to delete file",
			"key", key,
			"error", err,
		)
		return fmt.Errorf("%w: %v", domain.ErrBlobDeleteFailed, err)
	}

	f.logger.Debug("file deleted successfully", "key", key)
	return nil
}

// DeleteMultiple removes multiple objects from the file system.
func (f *FileSystemStore) DeleteMultiple(ctx context.Context, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	var failedKeys []string
	var mu sync.Mutex

	for _, key := range keys {
		// Check context periodically
		if err := ctx.Err(); err != nil {
			mu.Lock()
			failedKeys = append(failedKeys, keys...)
			mu.Unlock()
			return failedKeys, err
		}

		if err := f.Delete(ctx, key); err != nil && !errors.Is(err, domain.ErrBlobNotFound) {
			mu.Lock()
			failedKeys = append(failedKeys, key)
			mu.Unlock()
		}
	}

	if len(failedKeys) > 0 {
		return failedKeys, fmt.Errorf("%w: %d files failed to delete", domain.ErrBlobDeleteFailed, len(failedKeys))
	}

	f.logger.Debug("files deleted successfully", "count", len(keys))
	return nil, nil
}

// List lists objects in the file system store with optional filtering.
func (f *FileSystemStore) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	maxKeys := int(input.MaxKeys)
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	var objects []ObjectInfo
	prefix := input.Prefix
	startAfter := input.StartAfter

	err := filepath.WalkDir(f.basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Get relative key
		key, err := filepath.Rel(f.basePath, path)
		if err != nil {
			return nil // Skip files we can't get relative path for
		}

		// Use forward slashes for consistency
		key = filepath.ToSlash(key)

		// Apply prefix filter
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		// Apply startAfter filter
		if startAfter != "" && key <= startAfter {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil // Skip files we can't stat
		}

		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			ContentType:  detectContentType(key),
			LastModified: info.ModTime(),
		})

		return nil
	})

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		f.logger.Error("failed to list files",
			"prefix", input.Prefix,
			"error", err,
		)
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	// Sort by key for consistent ordering
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	// Apply maxKeys limit
	isTruncated := len(objects) > maxKeys
	if isTruncated {
		objects = objects[:maxKeys]
	}

	output := &ListOutput{
		Objects:     objects,
		IsTruncated: isTruncated,
	}

	if len(objects) > 0 {
		output.NextMarker = objects[len(objects)-1].Key
	}

	return output, nil
}

// Exists checks if an object exists in the file system.
func (f *FileSystemStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := f.HeadObject(ctx, key)
	if err != nil {
		if errors.Is(err, domain.ErrBlobNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Copy copies an object within the file system.
func (f *FileSystemStore) Copy(ctx context.Context, sourceKey, destKey string) error {
	if sourceKey == "" || destKey == "" {
		return domain.ErrInvalidBlobKey
	}

	sourcePath, err := f.fullPath(sourceKey)
	if err != nil {
		return err
	}

	destPath, err := f.fullPath(destKey)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Open source file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.ErrBlobNotFound
		}
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create destination directory if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy content
	if _, err := io.Copy(destFile, sourceFile); err != nil {
		os.Remove(destPath) // Clean up on error
		return fmt.Errorf("failed to copy file: %w", err)
	}

	f.logger.Debug("file copied successfully",
		"source", sourceKey,
		"dest", destKey,
	)
	return nil
}

// BasePath returns the base path of the file system store
func (f *FileSystemStore) BasePath() string {
	return f.basePath
}

// detectContentType attempts to detect content type based on file extension
func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	contentTypes := map[string]string{
		".html": "text/html",
		".htm":  "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".txt":  "text/plain",
		".md":   "text/markdown",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
		".ico":  "image/x-icon",
		".mp3":  "audio/mpeg",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".wasm": "application/wasm",
	}

	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}
