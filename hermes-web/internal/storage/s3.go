// Package storage keeps uploaded files in S3-compatible object storage (MinIO in dev). It uses only
// the plain S3 API, so any S3-compatible store (AWS S3, Garage, SeaweedFS, ...) can replace MinIO.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// partSize bounds memory per upload. With an unknown size, minio-go otherwise picks parts big
// enough for a 5 TiB object (~500 MiB each) and buffers a whole part in memory.
const partSize = 16 << 20

type S3 struct {
	client *minio.Client
	bucket string
}

// New connects to the store and creates the bucket if it doesn't exist.
func New(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL})
	if err != nil {
		return nil, fmt.Errorf("object storage client: %w", err)
	}
	ok, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket %q: %w", bucket, err)
	}
	if !ok {
		if err := c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %q: %w", bucket, err)
		}
	}
	return &S3{client: c, bucket: bucket}, nil
}

// Put streams r to key. The size isn't known up front, so it's sent as a multipart upload.
func (s *S3) Put(ctx context.Context, key, contentType string, r io.Reader) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, -1, minio.PutObjectOptions{ContentType: contentType, PartSize: partSize})
	if err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

// PresignGet returns a URL anyone can GET key from until ttl passes. The signature covers the
// host, so whoever fetches it must reach the store at the endpoint the API was configured with.
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", key, err)
	}
	return u.String(), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
