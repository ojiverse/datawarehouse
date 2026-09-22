package materializer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// ControlConfig selects the dedicated R2 control bucket that holds rebuild
// manifest, checkpoint, and chunk state
// (docs/architecture/cloudflare/processing/materialization.md, "Control Storage").
// It is deliberately a separate bucket/type from archive.Config and
// icebergcat.Config: control state is execution metadata, never a Source of
// Evidence, and never the Canonical Store itself.
type ControlConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	UsePathStyle    bool
}

// ControlStore is the durable state a resumable materializer run reads and
// writes: the immutable manifest, per-chunk checkpoints, and run status. It is
// an interface so the driver can be tested against an in-memory fake without
// a real R2/MinIO control bucket.
type ControlStore interface {
	// PutIfAbsentJSON writes v at key only if key does not already exist, and
	// reports whether this call created it. It is how the manifest is fixed
	// immutably at run start: a resumed run must load the existing manifest,
	// never silently regenerate (and thereby drift) it.
	PutIfAbsentJSON(ctx context.Context, key string, v any) (created bool, err error)
	// PutJSON writes v at key unconditionally (checkpoint/status updates are
	// idempotent re-derivations of the same state, not evidence).
	PutJSON(ctx context.Context, key string, v any) error
	// GetJSON reads key into v and reports whether it existed.
	GetJSON(ctx context.Context, key string, v any) (found bool, err error)
	// List returns all keys under prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}

// S3ControlStore is the production ControlStore backed by an S3-compatible
// bucket (R2 control bucket in production, MinIO in local/integration tests).
type S3ControlStore struct {
	s3     *s3.Client
	bucket string
	log    *slog.Logger
}

// NewControlStore builds a ControlStore with static credentials. Like
// archive.New, it never implicitly reads credentials from the process
// environment, so "no Discord credential in this runtime" can be asserted by
// the caller rather than by this package.
func NewControlStore(cfg ControlConfig, log *slog.Logger) (*S3ControlStore, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("materializer: control endpoint, bucket, access key id and secret are required")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	cl := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: cfg.UsePathStyle,
	})
	return &S3ControlStore{s3: cl, bucket: cfg.Bucket, log: log}, nil
}

// PutIfAbsentJSON implements ControlStore.PutIfAbsentJSON using S3
// If-None-Match, mirroring archive.Client.PutCreateOnly's create-only pattern.
func (s *S3ControlStore) PutIfAbsentJSON(ctx context.Context, key string, v any) (bool, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	_, err = s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
		IfNoneMatch: aws.String("*"),
	})
	if err == nil {
		return true, nil
	}
	if isPreconditionFailed(err) {
		return false, nil
	}
	return false, fmt.Errorf("control put-if-absent %s: %w", key, err)
}

// PutJSON implements ControlStore.PutJSON.
func (s *S3ControlStore) PutJSON(ctx context.Context, key string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("control put %s: %w", key, err)
	}
	return nil
}

// GetJSON implements ControlStore.GetJSON.
func (s *S3ControlStore) GetJSON(ctx context.Context, key string, v any) (bool, error) {
	out, err := s.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		if isNoSuchKey(err) {
			return false, nil
		}
		return false, fmt.Errorf("control get %s: %w", key, err)
	}
	defer out.Body.Close()
	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return false, fmt.Errorf("control read %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return false, fmt.Errorf("control decode %s: %w", key, err)
	}
	return true, nil
}

// List implements ControlStore.List.
func (s *S3ControlStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	p := s3.NewListObjectsV2Paginator(s.s3, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("control list %s: %w", prefix, err)
		}
		for _, o := range page.Contents {
			keys = append(keys, aws.ToString(o.Key))
		}
	}
	return keys, nil
}

func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "PreconditionFailed" || code == "ConditionalRequestConflict"
	}
	return strings.Contains(err.Error(), "412")
}

func isNoSuchKey(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "NoSuchKey" || code == "NotFound"
	}
	return false
}
