// Command dwh-materializer runs the resumable / idempotent Canonical
// materializer (Issue #40) as a standalone process, independent of any
// Cloudflare Workers runtime, per docs/architecture/cloudflare/processing/materialization.md.
//
// It reads only the Observation Archive and writes only the Canonical
// Iceberg table and its own rebuild control state; it never reads the
// existing Canonical Store or calls the Discord API, so a Full Rebuild works
// in a runtime with no Discord credential at all.
//
// All configuration comes from environment variables so no credential is
// ever written to disk or committed:
//
//	DWH_ARCHIVE_S3_ENDPOINT, DWH_ARCHIVE_BUCKET, DWH_ARCHIVE_REGION,
//	DWH_ARCHIVE_ACCESS_KEY_ID, DWH_ARCHIVE_SECRET_ACCESS_KEY, DWH_ARCHIVE_PATH_STYLE
//	DWH_CONTROL_S3_ENDPOINT, DWH_CONTROL_BUCKET, DWH_CONTROL_REGION,
//	DWH_CONTROL_ACCESS_KEY_ID, DWH_CONTROL_SECRET_ACCESS_KEY, DWH_CONTROL_PATH_STYLE
//	DWH_CATALOG_URI, DWH_CATALOG_WAREHOUSE, DWH_CATALOG_TOKEN, DWH_CATALOG_NAMESPACE,
//	DWH_CATALOG_PROPS (comma separated key=value FileIO properties)
//
// Usage:
//
//	dwh-materializer run [--run-id <uuid>] [--chunk-size N] [--commit-batch-size N]
//	                      [--extract-concurrency N] [--max-units N]
//	dwh-materializer status --run-id <uuid>
//
// `run` without --run-id starts a new rebuild (a fresh UUIDv7 Run ID is
// generated and printed). `run` with --run-id resumes that run: the
// Archive object manifest fixed by its first invocation is reloaded
// unchanged, and any chunk or commit batch already checkpointed is skipped.
// --max-units bounds how many chunks/commit-batches a single invocation
// performs new work on, so a run of any size can be driven to completion by
// repeated invocations without depending on one process's lifetime.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/canonical"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/materializer"
)

const defaultArchivePrefix = "observations/v1/source=http_backfill/"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("dwh-materializer failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: dwh-materializer <run|status> [flags]")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:], log)
	case "status":
		return statusCommand(ctx, args[1:], log)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCommand(ctx context.Context, args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	runID := fs.String("run-id", "", "resume this run (UUIDv7); empty starts a new run")
	chunkSize := fs.Int("chunk-size", 500, "Archive objects per manifest chunk")
	commitBatchSize := fs.Int("commit-batch-size", 5000, "Canonical rows per Iceberg commit / staging file")
	extractConcurrency := fs.Int("extract-concurrency", 8, "concurrent manifest chunks fetched/projected")
	maxUnits := fs.Int("max-units", 0, "stop after this many new chunks/commit-batches (0 = run to completion)")
	prefix := fs.String("archive-prefix", defaultArchivePrefix, "Archive key prefix to rebuild from")
	if err := fs.Parse(args); err != nil {
		return err
	}

	archiveCfg, err := archiveConfigFromEnv()
	if err != nil {
		return err
	}
	controlCfg, err := controlConfigFromEnv()
	if err != nil {
		return err
	}
	catalogCfg := catalogConfigFromEnv()

	archiveClient, err := archive.New(archiveCfg, log)
	if err != nil {
		return err
	}
	control, err := materializer.NewControlStore(controlCfg, log)
	if err != nil {
		return err
	}
	catCli, err := icebergcat.New(ctx, catalogCfg, log)
	if err != nil {
		return err
	}
	catalog := materializer.NewCatalogClient(catCli)

	id := *runID
	if id == "" {
		gen, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate run id: %w", err)
		}
		id = gen.String()
		log.Info("starting new rebuild run", slog.String("run_id", id))
	}

	cutoff := time.Now().UTC()
	objs, err := materializer.ListWithTimestamps(ctx, archiveCfg, *prefix)
	if err != nil {
		return fmt.Errorf("list archive objects: %w", err)
	}
	candidate := materializer.BuildManifest(id, *prefix, objs, cutoff, *chunkSize)

	cfg := materializer.RunConfig{
		RunID:                  id,
		CandidateManifest:      candidate,
		TableName:              canonical.TableName,
		CommitBatchSize:        *commitBatchSize,
		ExtractConcurrency:     *extractConcurrency,
		MaxUnitsThisInvocation: *maxUnits,
		Log:                    log,
	}
	status, err := materializer.Run(ctx, archiveClient, control, catalog, cfg)
	if err != nil {
		return err
	}
	return emitStatus(status)
}

func statusCommand(ctx context.Context, args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	runID := fs.String("run-id", "", "run to inspect (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("status requires --run-id")
	}
	controlCfg, err := controlConfigFromEnv()
	if err != nil {
		return err
	}
	control, err := materializer.NewControlStore(controlCfg, log)
	if err != nil {
		return err
	}
	var status materializer.RunStatus
	found, err := control.GetJSON(ctx, "runs/"+*runID+"/status.json", &status)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no status recorded yet for run %s", *runID)
	}
	return emitStatus(&status)
}

func emitStatus(status *materializer.RunStatus) error {
	b, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func archiveConfigFromEnv() (archive.Config, error) {
	cfg := archive.Config{
		Endpoint:        os.Getenv("DWH_ARCHIVE_S3_ENDPOINT"),
		Bucket:          os.Getenv("DWH_ARCHIVE_BUCKET"),
		Region:          os.Getenv("DWH_ARCHIVE_REGION"),
		AccessKeyID:     os.Getenv("DWH_ARCHIVE_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("DWH_ARCHIVE_SECRET_ACCESS_KEY"),
		UsePathStyle:    os.Getenv("DWH_ARCHIVE_PATH_STYLE") == "true",
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return cfg, fmt.Errorf("DWH_ARCHIVE_S3_ENDPOINT and DWH_ARCHIVE_BUCKET are required")
	}
	return cfg, nil
}

func controlConfigFromEnv() (materializer.ControlConfig, error) {
	cfg := materializer.ControlConfig{
		Endpoint:        os.Getenv("DWH_CONTROL_S3_ENDPOINT"),
		Bucket:          os.Getenv("DWH_CONTROL_BUCKET"),
		Region:          os.Getenv("DWH_CONTROL_REGION"),
		AccessKeyID:     os.Getenv("DWH_CONTROL_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("DWH_CONTROL_SECRET_ACCESS_KEY"),
		UsePathStyle:    os.Getenv("DWH_CONTROL_PATH_STYLE") == "true",
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return cfg, fmt.Errorf("DWH_CONTROL_S3_ENDPOINT and DWH_CONTROL_BUCKET are required")
	}
	return cfg, nil
}

func catalogConfigFromEnv() icebergcat.Config {
	return icebergcat.Config{
		URI:       os.Getenv("DWH_CATALOG_URI"),
		Warehouse: os.Getenv("DWH_CATALOG_WAREHOUSE"),
		Token:     os.Getenv("DWH_CATALOG_TOKEN"),
		Namespace: envOr("DWH_CATALOG_NAMESPACE", "dwh"),
		Props:     parseProps(os.Getenv("DWH_CATALOG_PROPS")),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseProps(s string) iceberg.Properties {
	props := iceberg.Properties{}
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			props[k] = v
		}
	}
	return props
}
