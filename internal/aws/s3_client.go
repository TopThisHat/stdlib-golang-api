package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/config"
	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Client provides operations for interacting with AWS S3.
// It wraps the AWS SDK v2 S3 client with domain-specific error handling
// and logging capabilities.
type S3Client struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucket     string
	logger     *logger.Logger
}

// S3ClientOption defines functional options for configuring S3Client
type S3ClientOption func(*s3ClientOptions)

type s3ClientOptions struct {
	// Upload configuration
	uploadPartSize      int64
	uploadConcurrency   int
	uploadLeavePartsErr bool

	// Download configuration
	downloadPartSize    int64
	downloadConcurrency int

	// Custom endpoint for testing (e.g., LocalStack, MinIO)
	customEndpoint string
	usePathStyle   bool
}

// defaultS3ClientOptions returns sensible defaults for S3 operations
func defaultS3ClientOptions() *s3ClientOptions {
	return &s3ClientOptions{
		uploadPartSize:      10 * 1024 * 1024, // 10 MB
		uploadConcurrency:   5,
		uploadLeavePartsErr: false,
		downloadPartSize:    10 * 1024 * 1024, // 10 MB
		downloadConcurrency: 5,
		usePathStyle:        false,
	}
}

// WithUploadPartSize sets the part size for multipart uploads (minimum 5MB)
func WithUploadPartSize(size int64) S3ClientOption {
	return func(o *s3ClientOptions) {
		if size >= 5*1024*1024 { // AWS minimum is 5MB
			o.uploadPartSize = size
		}
	}
}

// WithUploadConcurrency sets the number of concurrent upload goroutines
func WithUploadConcurrency(n int) S3ClientOption {
	return func(o *s3ClientOptions) {
		if n > 0 {
			o.uploadConcurrency = n
		}
	}
}

// WithDownloadPartSize sets the part size for multipart downloads
func WithDownloadPartSize(size int64) S3ClientOption {
	return func(o *s3ClientOptions) {
		if size > 0 {
			o.downloadPartSize = size
		}
	}
}

// WithDownloadConcurrency sets the number of concurrent download goroutines
func WithDownloadConcurrency(n int) S3ClientOption {
	return func(o *s3ClientOptions) {
		if n > 0 {
			o.downloadConcurrency = n
		}
	}
}

// WithCustomEndpoint sets a custom S3 endpoint (for LocalStack, MinIO, etc.)
func WithCustomEndpoint(endpoint string) S3ClientOption {
	return func(o *s3ClientOptions) {
		o.customEndpoint = endpoint
	}
}

// WithPathStyle enables path-style addressing (required for some S3-compatible services)
func WithPathStyle(enabled bool) S3ClientOption {
	return func(o *s3ClientOptions) {
		o.usePathStyle = enabled
	}
}

// NewS3Client creates a new S3 client with the provided configuration.
// It uses AWS SDK v2 with automatic credential resolution chain:
// 1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
// 2. Shared credentials file (~/.aws/credentials)
// 3. IAM role (for EC2/ECS/Lambda)
func NewS3Client(ctx context.Context, cfg *config.Config, log *logger.Logger, opts ...S3ClientOption) (*S3Client, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	options := defaultS3ClientOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Build AWS config options
	var awsOpts []func(*awsconfig.LoadOptions) error
	awsOpts = append(awsOpts, awsconfig.WithRegion(cfg.AWSRegion))

	// Use explicit credentials if provided, otherwise rely on default credential chain
	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		awsOpts = append(awsOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AWSAccessKeyID,
				cfg.AWSSecretAccessKey,
				"", // session token (optional)
			),
		))
	}

	// Load AWS configuration
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Build S3 client options
	var s3Opts []func(*s3.Options)
	if options.customEndpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(options.customEndpoint)
			o.UsePathStyle = options.usePathStyle
		})
	}

	// Create S3 client
	client := s3.NewFromConfig(awsCfg, s3Opts...)

	// Create upload manager with configured options
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = options.uploadPartSize
		u.Concurrency = options.uploadConcurrency
		u.LeavePartsOnError = options.uploadLeavePartsErr
	})

	// Create download manager with configured options
	downloader := manager.NewDownloader(client, func(d *manager.Downloader) {
		d.PartSize = options.downloadPartSize
		d.Concurrency = options.downloadConcurrency
	})

	log.Info("S3 client initialized",
		"bucket", cfg.S3Bucket,
		"region", cfg.AWSRegion,
	)

	return &S3Client{
		client:     client,
		uploader:   uploader,
		downloader: downloader,
		bucket:     cfg.S3Bucket,
		logger:     log,
	}, nil
}

// UploadInput contains parameters for uploading an object to S3
type UploadInput struct {
	Key         string            // Object key (required)
	Body        io.Reader         // Content to upload (required)
	ContentType string            // MIME type (optional, defaults to application/octet-stream)
	Metadata    map[string]string // Custom metadata (optional)
}

// UploadOutput contains the result of an upload operation
type UploadOutput struct {
	Location  string // URL of the uploaded object
	VersionID string // Version ID (if versioning is enabled)
	ETag      string // Entity tag for the object
}

// Upload uploads an object to S3 using multipart upload for large files.
// It automatically handles retries and chunking based on the configured part size.
func (c *S3Client) Upload(ctx context.Context, input *UploadInput) (*UploadOutput, error) {
	if input.Key == "" {
		return nil, domain.ErrInvalidBlobKey
	}

	if input.Body == nil {
		return nil, fmt.Errorf("%w: body is required", domain.ErrInvalidInput)
	}

	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	uploadInput := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(input.Key),
		Body:        input.Body,
		ContentType: aws.String(contentType),
	}

	if len(input.Metadata) > 0 {
		uploadInput.Metadata = input.Metadata
	}

	result, err := c.uploader.Upload(ctx, uploadInput)
	if err != nil {
		c.logger.Error("failed to upload object",
			"key", input.Key,
			"bucket", c.bucket,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobUploadFailed, err)
	}

	c.logger.Debug("object uploaded successfully",
		"key", input.Key,
		"location", result.Location,
	)

	output := &UploadOutput{
		Location: result.Location,
		ETag:     aws.ToString(result.ETag),
	}
	if result.VersionID != nil {
		output.VersionID = *result.VersionID
	}

	return output, nil
}

// Download downloads an object from S3 into the provided writer.
// It uses concurrent range requests for large files.
func (c *S3Client) Download(ctx context.Context, key string, w io.WriterAt) (int64, error) {
	if key == "" {
		return 0, domain.ErrInvalidBlobKey
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	n, err := c.downloader.Download(ctx, w, input)
	if err != nil {
		if c.isNotFoundError(err) {
			return 0, domain.ErrBlobNotFound
		}
		c.logger.Error("failed to download object",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return 0, fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}

	c.logger.Debug("object downloaded successfully",
		"key", key,
		"bytes", n,
	)

	return n, nil
}

// GetObject retrieves an object from S3 and returns it as a ReadCloser.
// The caller is responsible for closing the returned reader.
func (c *S3Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, domain.ErrInvalidBlobKey
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	result, err := c.client.GetObject(ctx, input)
	if err != nil {
		if c.isNotFoundError(err) {
			return nil, domain.ErrBlobNotFound
		}
		c.logger.Error("failed to get object",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrBlobDownloadFailed, err)
	}

	return result.Body, nil
}

// ObjectInfo contains metadata about an S3 object
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// HeadObject retrieves metadata about an object without downloading it.
func (c *S3Client) HeadObject(ctx context.Context, key string) (*ObjectInfo, error) {
	if key == "" {
		return nil, domain.ErrInvalidBlobKey
	}

	input := &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	result, err := c.client.HeadObject(ctx, input)
	if err != nil {
		if c.isNotFoundError(err) {
			return nil, domain.ErrBlobNotFound
		}
		c.logger.Error("failed to head object",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	info := &ObjectInfo{
		Key:         key,
		Size:        aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType),
		ETag:        aws.ToString(result.ETag),
		Metadata:    result.Metadata,
	}
	if result.LastModified != nil {
		info.LastModified = *result.LastModified
	}

	return info, nil
}

// Delete removes an object from S3.
func (c *S3Client) Delete(ctx context.Context, key string) error {
	if key == "" {
		return domain.ErrInvalidBlobKey
	}

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	_, err := c.client.DeleteObject(ctx, input)
	if err != nil {
		c.logger.Error("failed to delete object",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return fmt.Errorf("%w: %v", domain.ErrBlobDeleteFailed, err)
	}

	c.logger.Debug("object deleted successfully", "key", key)
	return nil
}

// DeleteMultiple removes multiple objects from S3 in a single request.
// It returns the keys that failed to delete along with any error.
func (c *S3Client) DeleteMultiple(ctx context.Context, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	// S3 DeleteObjects has a limit of 1000 keys per request
	const maxKeysPerRequest = 1000
	var failedKeys []string

	for i := 0; i < len(keys); i += maxKeysPerRequest {
		end := i + maxKeysPerRequest
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		objects := make([]types.ObjectIdentifier, len(batch))
		for j, key := range batch {
			objects[j] = types.ObjectIdentifier{
				Key: aws.String(key),
			}
		}

		input := &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		}

		result, err := c.client.DeleteObjects(ctx, input)
		if err != nil {
			c.logger.Error("failed to delete objects batch",
				"bucket", c.bucket,
				"count", len(batch),
				"error", err,
			)
			failedKeys = append(failedKeys, batch...)
			continue
		}

		// Collect failed deletions
		for _, errObj := range result.Errors {
			failedKeys = append(failedKeys, aws.ToString(errObj.Key))
			c.logger.Warn("failed to delete object",
				"key", aws.ToString(errObj.Key),
				"code", aws.ToString(errObj.Code),
				"message", aws.ToString(errObj.Message),
			)
		}
	}

	if len(failedKeys) > 0 {
		return failedKeys, fmt.Errorf("%w: %d objects failed to delete", domain.ErrBlobDeleteFailed, len(failedKeys))
	}

	c.logger.Debug("objects deleted successfully", "count", len(keys))
	return nil, nil
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

// List lists objects in the S3 bucket with optional filtering by prefix.
func (c *S3Client) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	maxKeys := input.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	listInput := &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		MaxKeys: aws.Int32(maxKeys),
	}

	if input.Prefix != "" {
		listInput.Prefix = aws.String(input.Prefix)
	}
	if input.StartAfter != "" {
		listInput.StartAfter = aws.String(input.StartAfter)
	}

	result, err := c.client.ListObjectsV2(ctx, listInput)
	if err != nil {
		c.logger.Error("failed to list objects",
			"bucket", c.bucket,
			"prefix", input.Prefix,
			"error", err,
		)
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	objects := make([]ObjectInfo, len(result.Contents))
	for i, obj := range result.Contents {
		objects[i] = ObjectInfo{
			Key:  aws.ToString(obj.Key),
			Size: aws.ToInt64(obj.Size),
			ETag: aws.ToString(obj.ETag),
		}
		if obj.LastModified != nil {
			objects[i].LastModified = *obj.LastModified
		}
	}

	output := &ListOutput{
		Objects:     objects,
		IsTruncated: aws.ToBool(result.IsTruncated),
	}

	if len(objects) > 0 {
		output.NextMarker = objects[len(objects)-1].Key
	}

	return output, nil
}

// GeneratePresignedURL generates a pre-signed URL for downloading an object.
// The URL is valid for the specified duration.
func (c *S3Client) GeneratePresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	if key == "" {
		return "", domain.ErrInvalidBlobKey
	}

	presignClient := s3.NewPresignClient(c.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiration))
	if err != nil {
		c.logger.Error("failed to generate presigned URL",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// GeneratePresignedUploadURL generates a pre-signed URL for uploading an object.
// The URL is valid for the specified duration.
func (c *S3Client) GeneratePresignedUploadURL(ctx context.Context, key string, contentType string, expiration time.Duration) (string, error) {
	if key == "" {
		return "", domain.ErrInvalidBlobKey
	}

	presignClient := s3.NewPresignClient(c.client)

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	request, err := presignClient.PresignPutObject(ctx, input, s3.WithPresignExpires(expiration))
	if err != nil {
		c.logger.Error("failed to generate presigned upload URL",
			"key", key,
			"bucket", c.bucket,
			"error", err,
		)
		return "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return request.URL, nil
}

// CopyObject copies an object within the same bucket or from another bucket.
func (c *S3Client) CopyObject(ctx context.Context, sourceKey, destKey string) error {
	if sourceKey == "" || destKey == "" {
		return domain.ErrInvalidBlobKey
	}

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(c.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", c.bucket, sourceKey)),
		Key:        aws.String(destKey),
	}

	_, err := c.client.CopyObject(ctx, input)
	if err != nil {
		if c.isNotFoundError(err) {
			return domain.ErrBlobNotFound
		}
		c.logger.Error("failed to copy object",
			"source", sourceKey,
			"dest", destKey,
			"bucket", c.bucket,
			"error", err,
		)
		return fmt.Errorf("failed to copy object: %w", err)
	}

	c.logger.Debug("object copied successfully",
		"source", sourceKey,
		"dest", destKey,
	)
	return nil
}

// Exists checks if an object exists in S3.
func (c *S3Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.HeadObject(ctx, key)
	if err != nil {
		if errors.Is(err, domain.ErrBlobNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// isNotFoundError checks if the error indicates the object was not found
func (c *S3Client) isNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}

	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}

	return false
}

// Bucket returns the configured bucket name
func (c *S3Client) Bucket() string {
	return c.bucket
}
