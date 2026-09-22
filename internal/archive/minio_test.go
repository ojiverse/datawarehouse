package archive_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/observation"
)

// localMinIOConfig matches spike/local/docker-compose.yml and
// spike/local/env.sh, the local stand-in for R2 this repo already uses for
// the #36 closed-loop spike. Tests here are gated the same way
// (internal/spike/spike_local_test.go): opt in with an environment variable
// once `docker compose -f spike/local/docker-compose.yml up -d minio
// minio-init` is running, so `go test ./...` skips them by default in CI and
// on a machine without Docker.
func localMinIOConfig(bucket string) archive.Config {
	return archive.Config{
		Endpoint: "http://localhost:9000", Region: "us-east-1", Bucket: bucket,
		AccessKeyID: "dwhspike", SecretAccessKey: "dwhspike-secret", UsePathStyle: true,
	}
}

func skipUnlessLocalMinIO(t *testing.T) {
	t.Helper()
	if os.Getenv("DWH_ARCHIVE_MINIO_TEST") != "1" {
		t.Skip("set DWH_ARCHIVE_MINIO_TEST=1 with `docker compose -f spike/local/docker-compose.yml up -d minio minio-init` running")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// rawPutBypassingClient uploads bytes directly through the S3 API, standing
// in for a producer (the TypeScript Worker, or a compliance rewrite) that
// wrote the object through a path other than archive.Client.PutCreateOnly. It
// lets the tests below exercise archive.Client purely as a replay reader
// against objects it did not itself write, which is the shape of the real
// TypeScript-writer / Go-reader boundary #38 asks to prove.
func rawPutBypassingClient(t *testing.T, cfg archive.Config, key string, body []byte, customMetadata map[string]string) {
	t.Helper()
	cl := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: cfg.UsePathStyle,
	})
	_, err := cl.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(cfg.Bucket), Key: aws.String(key), Body: bytes.NewReader(body),
		ContentType: aws.String("application/json"), ContentEncoding: aws.String("gzip"),
		Metadata: customMetadata,
	})
	if err != nil {
		t.Fatalf("raw put %s: %v", key, err)
	}
}

func deleteKeys(t *testing.T, arch *archive.Client, keys ...string) {
	t.Helper()
	for _, k := range keys {
		_ = arch.Delete(context.Background(), k)
	}
}

// TestReplayReaderListsAndDecodesRealTypeScriptObject stores the actual bytes
// the TypeScript producer wrote (see crosslanguage_test.go) into a real
// S3-compatible Observation Archive, then uses archive.Client - the same
// replay reader the Go materializer uses (internal/spike/spike.go) - to list
// the prefix and get the object back. This is the #38 cross-language
// integration proof end-to-end through the real write/list/get path, not
// just the decoder.
func TestReplayReaderListsAndDecodesRealTypeScriptObject(t *testing.T) {
	skipUnlessLocalMinIO(t)
	cfg := localMinIOConfig("dwh-observations")
	arch, err := archive.New(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	body, meta := loadTSProducedObject(t)
	t.Cleanup(func() { deleteKeys(t, arch, meta.Key) })

	rawPutBypassingClient(t, cfg, meta.Key, body, map[string]string{
		"format": meta.CustomMetadata.Format, "compression": meta.CustomMetadata.Compression,
		"envelope_version": meta.CustomMetadata.EnvelopeVersion, "observation_id": meta.CustomMetadata.ObservationID,
		"source_kind": meta.CustomMetadata.SourceKind, "payload_sha256": meta.CustomMetadata.PayloadSHA256,
	})

	prefix := "observations/v1/source=http_backfill/year=2026/month=09/day=21/"
	keys, err := arch.List(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range keys {
		if k == meta.Key {
			found = true
		}
	}
	if !found {
		t.Fatalf("List(%q) did not enumerate the TypeScript-produced object %q; got %v", prefix, meta.Key, keys)
	}

	env, err := arch.Get(ctx, meta.Key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got, want := env.ObservationID.String(), meta.ObservationID; got != want {
		t.Fatalf("observation_id = %q, want %q", got, want)
	}
	assertPayloadMatchesContractFixture(t, env.Payload)
}

func newHTTPEnvelope(t *testing.T, observedAt time.Time) observation.Envelope {
	t.Helper()
	id, err := observation.NewObservationID()
	if err != nil {
		t.Fatal(err)
	}
	return observation.Envelope{
		EnvelopeVersion: observation.EnvelopeVersion,
		ObservationID:   id,
		SourceKind:      observation.SourceHTTPBackfill,
		ObservedAt:      observation.TS(observedAt),
		Payload:         json.RawMessage(`[{"id":"1","channel_id":"2","author":{"id":"3"},"content":"hi","timestamp":"2026-09-22T00:00:00Z","edited_timestamp":null}]`),
		Provenance: &observation.HTTPProvenance{
			RunID: "0199a1b2-0000-7000-8000-00000000f1de", DiscordAPIVersion: "10",
			GuildID: "1", ChannelID: "2", Operation: "get_channel_messages", Endpoint: "/channels/2/messages",
			Pagination: observation.Pagination{Before: nil, After: nil}, Limit: 100,
			RequestStartedAt: observation.TS(observedAt), ResponseCompletedAt: observation.TS(observedAt), HTTPStatus: 200,
			Capabilities: map[string]bool{"message_content": true},
		},
	}
}

// TestArchiveWriteSemanticsAgainstRealStore proves, against a real
// S3-compatible store, the "No Overwrite" invariants of
// docs/architecture/cloudflare/observations/archive-write-path.md using the
// production archive.Client.PutCreateOnly / Get path: a retry does not
// overwrite, a same-id/same-hash retry is idempotent, and a same-key
// different-hash write is an invariant violation that leaves the original
// object untouched.
func TestArchiveWriteSemanticsAgainstRealStore(t *testing.T) {
	skipUnlessLocalMinIO(t)
	cfg := localMinIOConfig("dwh-observations")
	arch, err := archive.New(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	env := newHTTPEnvelope(t, time.Date(2099, 1, 2, 3, 0, 0, 0, time.UTC))
	key := env.ArchiveKey()
	t.Cleanup(func() { deleteKeys(t, arch, key) })

	res, err := arch.PutCreateOnly(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res != archive.PutCreated {
		t.Fatalf("first put = %s, want created", res)
	}

	got, err := arch.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.ObservationID != env.ObservationID {
		t.Fatalf("round trip changed observation id: got %s want %s", got.ObservationID, env.ObservationID)
	}

	// Same Observation ID + same source payload hash: idempotent retry, no overwrite.
	res, err = arch.PutCreateOnly(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	if res != archive.PutIdempotentRetry {
		t.Fatalf("same-id/same-hash retry = %s, want idempotent_retry", res)
	}

	// Same key (same Observed At + Observation ID), different payload hash:
	// invariant violation. Reuse the same ObservationID and ArchiveKey but
	// change the payload so the hash differs.
	conflicting := env
	conflicting.Payload = json.RawMessage(`[{"id":"999","channel_id":"2","author":{"id":"3"},"content":"different","timestamp":"2026-09-22T00:00:00Z","edited_timestamp":null}]`)
	if _, err := arch.PutCreateOnly(ctx, conflicting); err == nil {
		t.Fatal("expected an invariant violation error for a same-key different-hash write, got nil")
	}

	// The original object must be untouched by the rejected write.
	still, err := arch.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if still.PayloadSHA256() != env.PayloadSHA256() {
		t.Fatal("archive object was modified by a rejected invariant-violating write")
	}
}

// TestMalformedObjectIsListedButFailsToDecode proves the replay path never
// silently drops a broken object: List (which does not decode bodies) still
// enumerates it, and Get - the same call the Go materializer uses for every
// key it lists (internal/spike/spike.go materialize) - returns an error the
// caller must handle instead of a zero-value Envelope.
func TestMalformedObjectIsListedButFailsToDecode(t *testing.T) {
	skipUnlessLocalMinIO(t)
	cfg := localMinIOConfig("dwh-observations")
	arch, err := archive.New(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	id := uuid.Must(uuid.NewV7())
	key := fmt.Sprintf("observations/v1/source=http_backfill/year=2099/month=01/day=02/hour=04/%s.json.gz", id)
	t.Cleanup(func() { deleteKeys(t, arch, key) })

	// Not gzip at all: a corrupted / truncated write.
	rawPutBypassingClient(t, cfg, key, []byte("not-a-gzip-stream"), map[string]string{
		"format": "json", "compression": "gzip", "envelope_version": "v1",
		"observation_id": id.String(), "source_kind": "http_backfill",
	})

	prefix := "observations/v1/source=http_backfill/year=2099/month=01/day=02/"
	keys, err := arch.List(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range keys {
		if k == key {
			found = true
		}
	}
	if !found {
		t.Fatalf("List did not enumerate the malformed object %q; a replay pass would silently skip it", key)
	}

	if _, err := arch.Get(ctx, key); err == nil {
		t.Fatal("expected Get to fail decoding a malformed object, got nil error")
	}
}
