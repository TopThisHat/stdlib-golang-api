package blob

import (
	"context"
	"io"
	"time"
)

// ObjectInfo contains metadata about a stored object
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// UploadInput contains parameters for uploading an object
type UploadInput struct {
	Key         string            // Object key (required)
	Body        io.Reader         // Content to upload (required)
	ContentType string            // MIME type (optional, defaults to application/octet-stream)
	Metadata    map[string]string // Custom metadata (optional)
}

// UploadOutput contains the result of an upload operation
type UploadOutput struct {
	Location  string // URL or path of the uploaded object
	VersionID string // Version ID (if versioning is enabled)
	ETag      string // Entity tag for the object
}

// ListInput contains parameters for listing objects
type ListInput struct {
	Prefix     string // Filter objects by prefix
	MaxKeys    int32  // Maximum number of keys to return (default 1000)
	StartAfter string // Start listing after this key (for pagination)
}

// ListOutput contains the result of a list operation
type ListOutput struct {
	Objects     []ObjectInfo
	IsTruncated bool   // True if there are more results
	NextMarker  string // Use this as StartAfter for the next request
}

// Store defines the contract for blob storage operations.
// The domain defines the interface, infrastructure implements it.
// Implementations may include S3, GCS, Azure Blob, local filesystem, etc.
type Store interface {
	// Upload uploads an object to the store.
	// Returns upload metadata including location and ETag.
	Upload(ctx context.Context, input *UploadInput) (*UploadOutput, error)

	// Download downloads an object into the provided writer.
	// Returns the number of bytes written.
	Download(ctx context.Context, key string, w io.WriterAt) (int64, error)

	// GetObject retrieves an object and returns it as a ReadCloser.
	// The caller is responsible for closing the returned reader.
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)

	// HeadObject retrieves metadata about an object without downloading it.
	HeadObject(ctx context.Context, key string) (*ObjectInfo, error)

	// Delete removes an object from the store.
	Delete(ctx context.Context, key string) error

	// DeleteMultiple removes multiple objects from the store.
	// Returns the keys that failed to delete along with any error.
	DeleteMultiple(ctx context.Context, keys []string) (failedKeys []string, err error)

	// List lists objects in the store with optional filtering.
	List(ctx context.Context, input *ListInput) (*ListOutput, error)

	// Exists checks if an object exists in the store.
	Exists(ctx context.Context, key string) (bool, error)

	// Copy copies an object within the store.
	Copy(ctx context.Context, sourceKey, destKey string) error
}

// PresignedURLGenerator defines the contract for generating pre-signed URLs.
// Not all storage backends support this (e.g., local filesystem).
type PresignedURLGenerator interface {
	// GeneratePresignedURL generates a pre-signed URL for downloading an object.
	// The URL is valid for the specified duration.
	GeneratePresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// GeneratePresignedUploadURL generates a pre-signed URL for uploading an object.
	// The URL is valid for the specified duration.
	GeneratePresignedUploadURL(ctx context.Context, key string, contentType string, expiration time.Duration) (string, error)
}

// FullStore combines Store with PresignedURLGenerator for backends that support both.
type FullStore interface {
	Store
	PresignedURLGenerator
}
