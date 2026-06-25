package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Options configures an S3 (or S3-compatible) Store. Credentials are NOT here:
// the SDK resolves them from the standard chain (env vars, shared config,
// instance/role), keeping the siphon config file free of secrets.
type S3Options struct {
	Bucket   string
	Prefix   string // optional key prefix within the bucket
	Region   string
	Endpoint string // optional custom endpoint for S3-compatible services (MinIO, R2)
}

// s3Store is a Store backed by an S3 bucket. Put streams via the multipart
// upload manager (atomic on completion — the key does not resolve until the
// upload finishes), so a failed/cancelled upload leaves no partial object.
type s3Store struct {
	client   *s3.Client
	uploader *transfermanager.Client
	bucket   string
	prefix   string
}

// NewS3 builds an S3-backed Store. It loads AWS config (region, credentials)
// from the default chain and applies the optional custom endpoint for
// S3-compatible services. The bucket is assumed to exist.
func NewS3(ctx context.Context, opt S3Options) (Store, error) {
	if opt.Bucket == "" {
		return nil, errors.New("storage.s3: bucket is required")
	}
	var loadOpts []func(*awsconfig.LoadOptions) error
	if opt.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(opt.Region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("storage.s3: load aws config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opt.Endpoint != "" {
			o.BaseEndpoint = aws.String(opt.Endpoint)
			o.UsePathStyle = true // S3-compatible services (MinIO) need path-style
		}
	})
	return &s3Store{
		client:   client,
		uploader: transfermanager.New(client),
		bucket:   opt.Bucket,
		prefix:   strings.TrimSuffix(opt.Prefix, "/"),
	}, nil
}

func (s *s3Store) objectKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

func (s *s3Store) Put(ctx context.Context, key string, r io.Reader) error {
	_, err := s.uploader.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("storage.s3: put %s: %w", key, err)
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, fmt.Errorf("storage.s3: %s: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage.s3: get %s: %w", key, err)
	}
	return out.Body, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("storage.s3: delete %s: %w", key, err)
	}
	return nil // idempotent
}

func (s *s3Store) List(ctx context.Context) ([]string, error) {
	var keys []string
	prefix := ""
	if s.prefix != "" {
		prefix = s.prefix + "/"
	}
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage.s3: list: %w", err)
		}
		for _, obj := range page.Contents {
			k := aws.ToString(obj.Key)
			keys = append(keys, strings.TrimPrefix(k, prefix))
		}
	}
	return keys, nil
}

func (s *s3Store) Stat(ctx context.Context, key string) (int64, bool, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(key)),
	})
	if err != nil {
		if isS3NotFound(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("storage.s3: stat %s: %w", key, err)
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

// isS3NotFound reports whether err is S3's "no such key / not found". GetObject
// returns *types.NoSuchKey; HeadObject returns a generic *types.NotFound (it has
// no typed NoSuchKey). Both are matched here so missing-object handling is
// uniform across operations.
func isS3NotFound(err error) bool {
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *s3types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	// S3-compatible services sometimes surface a bare API error code instead of
	// the typed shape; match the canonical codes as a fallback.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
