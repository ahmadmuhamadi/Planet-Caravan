package store

import (
	"context"
	"errors"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Blobs reads the frame images compute wrote to object storage.
type Blobs interface {
	// Open returns the object and its size, or ErrNotFound.
	Open(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Ping(ctx context.Context) error
}

// S3 reads from S3-compatible object storage (MinIO in local dev).
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 connects to the bucket at endpoint, an http(s) URL.
func NewS3(endpoint, region, bucket, accessKey, secretKey string) (*S3, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, errors.New("S3_ENDPOINT must be an http(s) URL")
	}
	c, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: u.Scheme == "https",
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{client: c, bucket: bucket}, nil
}

// Open implements Blobs.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	info, err := obj.Stat() // GetObject is lazy; Stat surfaces a missing key
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	return obj, info.Size, nil
}

// Ping implements Blobs.
func (s *S3) Ping(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err == nil && !ok {
		err = errors.New("bucket " + s.bucket + " does not exist")
	}
	return err
}
