// Package s3 implements the invoice object store on any S3-compatible service
// (MinIO locally, AWS S3 later) with the minio-go client. The bucket is private:
// clients never receive a URL to it, downloads go through the authenticated API.
package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
)

// Config selects the S3 endpoint and credentials. Endpoint is host:port
// without a scheme; UseSSL selects https.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

// Store is an app.ObjectStore on one bucket.
type Store struct {
	client *minio.Client
	bucket string
	region string
}

var _ app.ObjectStore = (*Store)(nil)

// New builds a Store. It does not contact the server; call EnsureBucket at startup.
func New(cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &Store{client: client, bucket: cfg.Bucket, region: cfg.Region}, nil
}

// EnsureBucket creates the bucket when it does not exist. New buckets are
// private (no policy is attached).
func (s *Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: s.region}); err != nil {
		// Another process may have created it in the meantime.
		if code := minio.ToErrorResponse(err).Code; code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists" {
			return nil
		}
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

// Put stores size bytes from r under key.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Get returns the object body. GetObject is lazy, so the object is statted
// first to report a missing key as app.ErrObjectNotFound.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, app.ErrObjectNotFound
		}
		return nil, err
	}
	return obj, nil
}

// Delete removes the object. S3 answers success for a missing key.
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return nil
	}
	return err
}
