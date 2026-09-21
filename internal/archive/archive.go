// Package archive writes and reads Observation Archive objects on R2 through
// the S3-compatible API, following docs/architecture/cloudflare/observations/archive-write-path.md.
package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// Metadata keys stored as S3 user metadata so each object is self-describing.
const (
	metaFormat          = "format"
	metaCompression     = "compression"
	metaEnvelopeVersion = "envelope_version"
	metaObservationID   = "observation_id"
	metaSourceKind      = "source_kind"
	metaPayloadSHA256   = "payload_sha256"
)

// Config selects the bucket and S3 endpoint (R2 or a local S3-compatible store).
type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	// UsePathStyle is required for MinIO; R2 accepts both.
	UsePathStyle bool
}

// Client is a thin Archive-specific wrapper over the S3 client.
type Client struct {
	s3     *s3.Client
	bucket string
	log    *slog.Logger
}

// New builds a client with static credentials; no credential is read from
// the process environment implicitly so a "no Discord / no Cloudflare secret"
// rebuild environment can be asserted by the caller.
func New(cfg Config, log *slog.Logger) (*Client, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("archive: endpoint, bucket, access key id and secret are required")
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
	return &Client{s3: cl, bucket: cfg.Bucket, log: log}, nil
}

// PutResult reports how a create-only put ended.
type PutResult string

// Outcomes of PutCreateOnly.
const (
	PutCreated         PutResult = "created"
	PutIdempotentRetry PutResult = "idempotent_retry"
)

// PutCreateOnly stores an Envelope with `If-None-Match: *`. If the key exists,
// the stored Observation ID and payload hash must match; a mismatch is an
// invariant violation and is returned as an error without touching the object.
func (c *Client) PutCreateOnly(ctx context.Context, env observation.Envelope) (PutResult, error) {
	body, err := observation.EncodeGzipJSON(env)
	if err != nil {
		return "", err
	}
	key := env.ArchiveKey()
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(c.bucket),
		Key:             aws.String(key),
		Body:            bytes.NewReader(body),
		ContentType:     aws.String("application/json"),
		ContentEncoding: aws.String("gzip"),
		IfNoneMatch:     aws.String("*"),
		Metadata: map[string]string{
			metaFormat: "json", metaCompression: "gzip",
			metaEnvelopeVersion: env.EnvelopeVersion,
			metaObservationID:   env.ObservationID.String(),
			metaSourceKind:      string(env.SourceKind),
			metaPayloadSHA256:   env.PayloadSHA256(),
		},
	})
	if err == nil {
		c.log.Debug("archive object created", slog.String("key", key))
		return PutCreated, nil
	}
	if !isPreconditionFailed(err) {
		return "", fmt.Errorf("put %s: %w", key, err)
	}
	head, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return "", fmt.Errorf("head existing %s: %w", key, err)
	}
	if head.Metadata[metaObservationID] != env.ObservationID.String() || head.Metadata[metaPayloadSHA256] != env.PayloadSHA256() {
		return "", fmt.Errorf("archive invariant violation: key %s exists with different observation id or payload hash", key)
	}
	return PutIdempotentRetry, nil
}

func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "PreconditionFailed" || code == "ConditionalRequestConflict"
	}
	return strings.Contains(err.Error(), "412")
}

// List returns all object keys under prefix, in the order the store returns.
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	p := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{Bucket: aws.String(c.bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, o := range page.Contents {
			keys = append(keys, aws.ToString(o.Key))
		}
	}
	return keys, nil
}

// Get downloads and decodes one Envelope.
func (c *Client) Get(ctx context.Context, key string) (observation.Envelope, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return observation.Envelope{}, fmt.Errorf("get %s: %w", key, err)
	}
	defer out.Body.Close()
	// R2 may transparently decode Content-Encoding: gzip depending on client;
	// the SDK does not, so read raw bytes and let the decoder handle gzip.
	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return observation.Envelope{}, fmt.Errorf("read %s: %w", key, err)
	}
	return observation.DecodeGzipJSON(bytes.NewReader(raw))
}

// Delete removes one object (used only for spike cleanup, never by ingestion).
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}
