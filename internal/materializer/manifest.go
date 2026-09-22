// Package materializer promotes the #36 spike (internal/canonical,
// internal/icebergcat, internal/projection, internal/r2sql) into a
// production-shaped resumable / idempotent Canonical materializer, per
// docs/architecture/cloudflare/processing/materialization.md.
//
// It does not reimplement the Iceberg commit, Parquet writing, or projection
// logic that #36/#39 already established; it adds the durable manifest,
// chunking, checkpoint, and crash-recovery orchestration that turns that
// spike code into a resumable rebuild engine.
package materializer

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ojiverse/datawarehouse/internal/archive"
)

// ObjectRef is one Archive object as seen by manifest construction: its key
// and the time R2 recorded the object as uploaded. UploadedAt is rebuild
// control metadata only, never Observation identity (docs/domain/observations/identity.md
// forbids defining identity from physical placement).
type ObjectRef struct {
	Key        string
	UploadedAt time.Time
}

// Manifest is the immutable rebuild input fixed at run start: the Archive
// object set, split into resumable chunks in a deterministic order. It is
// rebuild control metadata, not a Source of Evidence
// (docs/architecture/cloudflare/processing/materialization.md, "Rebuild Input Snapshot").
type Manifest struct {
	RunID  string     `json:"run_id"`
	Prefix string     `json:"prefix"`
	Cutoff time.Time  `json:"cutoff"`
	Chunks [][]string `json:"chunks"`
}

// BuildManifest fixes the input Archive object set at rebuild start: objects
// uploaded at or before cutoff are kept, everything else (in particular
// anything written concurrently with the run) is excluded. The kept keys are
// sorted and split into contiguous chunks of chunkSize so the split itself is
// a pure function of the (cutoff-filtered) key set, independent of listing
// order or the number of times the manifest is rebuilt from the same input.
func BuildManifest(runID, prefix string, objs []ObjectRef, cutoff time.Time, chunkSize int) Manifest {
	if chunkSize < 1 {
		chunkSize = 1
	}
	keys := make([]string, 0, len(objs))
	for _, o := range objs {
		if !o.UploadedAt.After(cutoff) {
			keys = append(keys, o.Key)
		}
	}
	sort.Strings(keys)

	var chunks [][]string
	for i := 0; i < len(keys); i += chunkSize {
		end := i + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := make([]string, end-i)
		copy(chunk, keys[i:end])
		chunks = append(chunks, chunk)
	}
	return Manifest{RunID: runID, Prefix: prefix, Cutoff: cutoff.UTC(), Chunks: chunks}
}

// ListWithTimestamps lists Archive object keys and their upload time under
// prefix. archive.Client.List (a fixed dependency, see AGENTS.md) only
// returns keys, so this opens its own S3 client from the same archive.Config
// fields rather than modifying that package's public API or contract.
func ListWithTimestamps(ctx context.Context, cfg archive.Config, prefix string) ([]ObjectRef, error) {
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
	var out []ObjectRef
	p := s3.NewListObjectsV2Paginator(cl, &s3.ListObjectsV2Input{Bucket: aws.String(cfg.Bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, o := range page.Contents {
			if o.Key == nil || o.LastModified == nil {
				continue
			}
			out = append(out, ObjectRef{Key: aws.ToString(o.Key), UploadedAt: o.LastModified.UTC()})
		}
	}
	return out, nil
}

// LogChunkSizes reports a manifest's shape for operational visibility.
func LogChunkSizes(log *slog.Logger, m Manifest) {
	log.Info("manifest built",
		slog.String("run_id", m.RunID),
		slog.Int("chunk_count", len(m.Chunks)),
		slog.Time("cutoff", m.Cutoff))
}
