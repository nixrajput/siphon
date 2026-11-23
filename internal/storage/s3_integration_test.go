//go:build integration

package storage

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

// TestS3Store_Contract runs the full Store contract suite against a real MinIO
// (S3-compatible) container, so the streaming multipart Put, GetObject Get,
// ListObjectsV2, HeadObject Stat, and not-found mapping all execute against a
// live object store — not just compile. Each subtest gets a fresh uniquely
// prefixed view of one shared bucket, so the suite's "empty store" assumption
// holds without standing up a container per subtest.
func TestS3Store_Contract(t *testing.T) {
	ctx := context.Background()

	// minio/minio is gone from Docker Hub. Chainguard's MinIO build has only a moving
	// latest tag, so pin its index digest (taken 2026-11-23).
	mc, err := tcminio.Run(ctx, "cgr.dev/chainguard/minio@sha256:bd92a45b821c0d85772c607ed7b49693efe78b3ffffae301b5d1b0981660de22")
	if err != nil {
		t.Fatalf("start minio: %v", err)
	}
	t.Cleanup(func() { _ = mc.Terminate(ctx) })

	endpoint, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("minio endpoint: %v", err)
	}
	endpointURL := "http://" + endpoint

	// MinIO credentials come from the container; feed them to the SDK via env so
	// NewS3's default credential chain picks them up.
	t.Setenv("AWS_ACCESS_KEY_ID", mc.Username)
	t.Setenv("AWS_SECRET_ACCESS_KEY", mc.Password)
	t.Setenv("AWS_REGION", "us-east-1")

	const bucket = "siphon-test"
	createBucket(t, ctx, endpointURL, mc.Username, mc.Password, bucket)

	prefixN := 0
	RunStoreSuite(t, func(t *testing.T) Store {
		// Unique prefix per subtest → a fresh empty view of the shared bucket.
		prefixN++
		st, err := NewS3(ctx, S3Options{
			Bucket:   bucket,
			Prefix:   "sub" + string(rune('a'+prefixN)),
			Region:   "us-east-1",
			Endpoint: endpointURL,
		})
		if err != nil {
			t.Fatalf("NewS3: %v", err)
		}
		return st
	})
}

// createBucket provisions the test bucket on the fresh MinIO instance.
func createBucket(t *testing.T, ctx context.Context, endpoint, user, pass, bucket string) {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(user, pass, "")),
	)
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
}
