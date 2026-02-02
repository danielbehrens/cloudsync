package storage

import (
	"context"
	"time"
)

// Adapter wraps S3Client to implement sync.Storage interface
type Adapter struct {
	client *S3Client
}

// NewAdapter creates a new storage adapter
func NewAdapter(client *S3Client) *Adapter {
	return &Adapter{client: client}
}

// Upload implements sync.Storage
func (a *Adapter) Upload(ctx context.Context, localPath, objectName string) error {
	return a.client.Upload(ctx, localPath, objectName)
}

// Download implements sync.Storage
func (a *Adapter) Download(ctx context.Context, objectName, localPath string) error {
	return a.client.Download(ctx, objectName, localPath)
}

// Stat implements sync.Storage
func (a *Adapter) Stat(ctx context.Context, objectName string) (*SyncFileInfo, error) {
	info, err := a.client.Stat(ctx, objectName)
	if err != nil {
		return nil, err
	}

	return &SyncFileInfo{
		Name:    info.Name,
		ModTime: info.ModTime,
		Size:    info.Size,
	}, nil
}

// List implements sync.Storage
func (a *Adapter) List(ctx context.Context) ([]*SyncFileInfo, error) {
	files, err := a.client.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []*SyncFileInfo
	for _, f := range files {
		result = append(result, &SyncFileInfo{
			Name:    f.Name,
			ModTime: f.ModTime,
			Size:    f.Size,
		})
	}

	return result, nil
}

// EnsureBucket implements sync.Storage
func (a *Adapter) EnsureBucket(ctx context.Context) error {
	return a.client.EnsureBucket(ctx)
}

// SyncFileInfo is the file info type used by sync package
type SyncFileInfo struct {
	Name    string
	ModTime time.Time
	Size    int64
}
