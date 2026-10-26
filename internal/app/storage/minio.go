package storage

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Remove(context.Context, string) error
	URL(string) string
}

type MinIO struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

func New() (*MinIO, error) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "media"
	}
	access, secret := os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	if access == "" || secret == "" {
		return nil, errors.New("задайте MINIO_ACCESS_KEY и MINIO_SECRET_KEY")
	}
	secure := os.Getenv("MINIO_SECURE") == "true"
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: secure, MaxRetries: 2})
	if err != nil {
		return nil, err
	}
	publicURL := os.Getenv("MINIO_PUBLIC_URL")
	if publicURL == "" {
		scheme := "http://"
		if secure {
			scheme = "https://"
		}
		publicURL = scheme + endpoint + "/" + bucket
	}
	return &MinIO{client: client, bucket: bucket, publicURL: strings.TrimRight(publicURL, "/")}, nil
}

func (s *MinIO) Check(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("бакет MinIO не существует")
	}
	return nil
}

func (s *MinIO) Put(ctx context.Context, key string, source io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, source, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *MinIO) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *MinIO) URL(key string) string {
	if key == "" {
		return ""
	}
	return s.publicURL + "/" + url.PathEscape(key)
}
