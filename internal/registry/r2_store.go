package registry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/calebfaruki/impromptu/internal/oci"
)

// R2Store stores blobs in Cloudflare R2 (S3-compatible).
type R2Store struct {
	client *s3.Client
	bucket string
}

// NewR2Store creates a blob store backed by Cloudflare R2.
func NewR2Store(endpoint, accessKeyID, secretAccessKey, bucket string) (*R2Store, error) {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	})
	return &R2Store{client: client, bucket: bucket}, nil
}

func (r *R2Store) Put(ctx context.Context, digest oci.Digest, data []byte) error {
	if err := digest.Validate(); err != nil {
		return fmt.Errorf("invalid digest: %w", err)
	}
	key := blobKey(digest)
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("uploading blob %s: %w", digest, err)
	}
	return nil
}

func (r *R2Store) Get(ctx context.Context, digest oci.Digest) ([]byte, error) {
	if err := digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid digest: %w", err)
	}
	key := blobKey(digest)
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("downloading blob %s: %w", digest, err)
	}
	defer result.Body.Close()
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("reading blob %s: %w", digest, err)
	}
	return data, nil
}

func (r *R2Store) Exists(ctx context.Context, digest oci.Digest) (bool, error) {
	if err := digest.Validate(); err != nil {
		return false, fmt.Errorf("invalid digest: %w", err)
	}
	key := blobKey(digest)
	_, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking blob %s: %w", digest, err)
	}
	return true, nil
}

func blobKey(digest oci.Digest) string {
	hex := digest.Hex()
	return "sha256/" + hex[:2] + "/" + hex
}

func isNotFoundErr(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	return errors.As(err, &nf)
}
