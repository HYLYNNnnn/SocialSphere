package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

const gcsOperationTimeout = 60 * time.Second

// GoogleCloudStorageBackend keeps objects private. The application serves media
// through its own HTTP route rather than changing bucket or object ACLs.
type GoogleCloudStorageBackend struct {
	client *storage.Client
	bucket string
}

func NewGCSBackend(ctx context.Context, bucket string) (*GoogleCloudStorageBackend, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("a Google Cloud Storage bucket is required")
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize Google Cloud Storage: %w", err)
	}
	return &GoogleCloudStorageBackend{client: client, bucket: bucket}, nil
}

func (b *GoogleCloudStorageBackend) Close() error {
	if b == nil || b.client == nil {
		return nil
	}
	return b.client.Close()
}

func (b *GoogleCloudStorageBackend) SaveMedia(ctx context.Context, name, contentType string, reader io.Reader) error {
	if name == "" || contentType == "" || reader == nil {
		return errors.New("media name, content type and reader are required")
	}
	ctx, cancel := context.WithTimeout(ctx, gcsOperationTimeout)
	defer cancel()
	// A post's generated object ID is immutable; a collision must not replace media.
	object := b.client.Bucket(b.bucket).Object(name).If(storage.Conditions{DoesNotExist: true})
	writer := object.NewWriter(ctx)
	writer.ContentType = contentType
	writer.ChunkSize = 1024 * 1024
	if _, err := io.Copy(writer, reader); err != nil {
		// Cancel before Close so a partial body cannot be finalized as an object.
		cancel()
		_ = writer.Close()
		return fmt.Errorf("write media: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish media upload: %w", err)
	}
	return nil
}

func (b *GoogleCloudStorageBackend) OpenMedia(ctx context.Context, name string) (io.ReadCloser, string, error) {
	if name == "" {
		return nil, "", fs.ErrNotExist
	}
	readContext, cancel := context.WithTimeout(ctx, gcsOperationTimeout)
	reader, err := b.client.Bucket(b.bucket).Object(name).NewReader(readContext)
	if err != nil {
		cancel()
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, "", fs.ErrNotExist
		}
		return nil, "", fmt.Errorf("open media: %w", err)
	}
	contentType := reader.Attrs.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Keep the context alive until the caller has finished streaming the body.
	return &gcsMediaReader{ReadCloser: reader, cancel: cancel}, contentType, nil
}

type gcsMediaReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *gcsMediaReader) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}

func (b *GoogleCloudStorageBackend) DeleteMedia(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, gcsOperationTimeout)
	defer cancel()
	err := b.client.Bucket(b.bucket).Object(name).Delete(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	return nil
}
