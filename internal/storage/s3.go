package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Client wraps MinIO client for S3 operations
type S3Client struct {
	client     *minio.Client
	bucketName string
}

// FileInfo represents metadata about a file in storage
type FileInfo struct {
	Name        string
	ModTime     time.Time
	Size        int64
	ETag        string
}

// NewS3Client creates a new S3 client
func NewS3Client(endpoint, accessKey, secretKey, bucketName string, useSSL bool) (*S3Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &S3Client{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// EnsureBucket ensures the bucket exists, creating it if necessary
func (s *S3Client) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return nil
}

// Upload uploads a file to S3 with metadata
func (s *S3Client) Upload(ctx context.Context, localPath, objectName string) error {
	// Get file mod time
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	modTime := info.ModTime().UTC()

	// Store full Unix nanoseconds timestamp in metadata
	userMeta := map[string]string{
		"X-Amz-Meta-Modtime":       fmt.Sprintf("%d", modTime.UnixNano()),
		"X-Amz-Meta-ModtimeString": modTime.Format("2006-01-02_15-04-05.000000"),
	}

	_, err = s.client.FPutObject(ctx, s.bucketName, objectName, localPath, minio.PutObjectOptions{
		UserMetadata: userMeta,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

// Download downloads a file from S3
func (s *S3Client) Download(ctx context.Context, objectName, localPath string) error {
	err := s.client.FGetObject(ctx, s.bucketName, objectName, localPath, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

// Stat retrieves metadata about an object in S3
func (s *S3Client) Stat(ctx context.Context, objectName string) (*FileInfo, error) {
	stat, err := s.client.StatObject(ctx, s.bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	modTime := extractModTime(stat)

	return &FileInfo{
		Name:    stat.Key,
		ModTime: modTime,
		Size:    stat.Size,
		ETag:    stat.ETag,
	}, nil
}

// List returns all objects in the bucket
func (s *S3Client) List(ctx context.Context) ([]*FileInfo, error) {
	var files []*FileInfo

	objectCh := s.client.ListObjects(ctx, s.bucketName, minio.ListObjectsOptions{Recursive: true})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing objects: %w", object.Err)
		}

		// Fetch full metadata (including custom mod time)
		stat, err := s.client.StatObject(ctx, s.bucketName, object.Key, minio.StatObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to stat object %s: %w", object.Key, err)
		}

		modTime := extractModTime(stat)

		files = append(files, &FileInfo{
			Name:    object.Key,
			ModTime: modTime,
			Size:    object.Size,
			ETag:    object.ETag,
		})
	}

	return files, nil
}

// extractModTime extracts modification time from S3 object metadata
func extractModTime(stat minio.ObjectInfo) time.Time {
	cloudModTime := stat.LastModified

	rawModTime := stat.UserMetadata["Modtime"]
	if rawModTime != "" {
		ts, err := strconv.ParseInt(rawModTime, 10, 64)
		if err == nil {
			cloudModTime = time.Unix(0, ts)
		}
	}

	return cloudModTime.UTC()
}

// CopyFile is a utility function to copy files locally
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

// EnsureDir ensures a directory exists, creating it if necessary
func EnsureDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory: %s", path)
	}
	return nil
}
