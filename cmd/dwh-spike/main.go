// Command dwh-spike runs the Issue #36 technical spike against an Iceberg REST
// Catalog (R2 Data Catalog or a local catalog) and prints a JSON report.
//
// All configuration comes from environment variables so that no credential is
// ever written to disk or committed:
//
//	DWH_ARCHIVE_S3_ENDPOINT, DWH_ARCHIVE_BUCKET, DWH_ARCHIVE_ACCESS_KEY_ID,
//	DWH_ARCHIVE_SECRET_ACCESS_KEY, DWH_ARCHIVE_PATH_STYLE (true for MinIO)
//	DWH_CATALOG_URI, DWH_CATALOG_WAREHOUSE, DWH_CATALOG_TOKEN, DWH_CATALOG_NAMESPACE,
//	DWH_CATALOG_PROPS (comma separated key=value FileIO properties)
//	DWH_R2SQL_ACCOUNT_ID, DWH_R2SQL_BUCKET, DWH_R2SQL_TOKEN (unset → R2 SQL criteria skipped)
//	DWH_SPIKE_REPORT (path of the JSON report; default stdout only)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/apache/iceberg-go"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/r2sql"
	"github.com/ojiverse/datawarehouse/internal/spike"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("dwh-spike failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: dwh-spike <loop|cleanup>")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch args[0] {
	case "loop":
		rep, runErr := spike.Run(ctx, cfg, log)
		if err := emitReport(rep); err != nil {
			return err
		}
		if runErr != nil {
			return runErr
		}
		if !rep.AllPassed() {
			return fmt.Errorf("some criteria failed")
		}
		return nil
	case "cleanup":
		return spike.Cleanup(ctx, cfg, log)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func emitReport(rep *spike.Report) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	if p := os.Getenv("DWH_SPIKE_REPORT"); p != "" {
		return os.WriteFile(p, b, 0o644)
	}
	return nil
}

func configFromEnv() (spike.Config, error) {
	cfg := spike.Config{
		Archive: archive.Config{
			Endpoint:        os.Getenv("DWH_ARCHIVE_S3_ENDPOINT"),
			Bucket:          os.Getenv("DWH_ARCHIVE_BUCKET"),
			Region:          os.Getenv("DWH_ARCHIVE_REGION"),
			AccessKeyID:     os.Getenv("DWH_ARCHIVE_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("DWH_ARCHIVE_SECRET_ACCESS_KEY"),
			UsePathStyle:    os.Getenv("DWH_ARCHIVE_PATH_STYLE") == "true",
		},
		ArchivePrefix: "observations/v1/source=http_backfill/",
		Catalog: icebergcat.Config{
			URI:       os.Getenv("DWH_CATALOG_URI"),
			Warehouse: os.Getenv("DWH_CATALOG_WAREHOUSE"),
			Token:     os.Getenv("DWH_CATALOG_TOKEN"),
			Namespace: envOr("DWH_CATALOG_NAMESPACE", "dwh_spike"),
			Props:     parseProps(os.Getenv("DWH_CATALOG_PROPS")),
		},
		TableName:          "message",
		Fixture:            observation.DefaultFixtureSpec(),
		ContractFixtureDir: envOr("DWH_SPIKE_CONTRACT_DIR", "contracts/observation-envelope/v1"),
		DiscordEnvKeys:     []string{"DISCORD_BOT_TOKEN", "DISCORD_TOKEN", "DISCORD_CLIENT_SECRET"},
	}
	cfg.R2SQLTable = cfg.Catalog.Namespace + "." + cfg.TableName
	if acct := os.Getenv("DWH_R2SQL_ACCOUNT_ID"); acct != "" {
		cfg.R2SQL = &r2sql.Config{
			AccountID: acct,
			Bucket:    envOr("DWH_R2SQL_BUCKET", os.Getenv("DWH_CATALOG_WAREHOUSE")),
			Token:     envOr("DWH_R2SQL_TOKEN", cfg.Catalog.Token),
		}
	}
	return cfg, nil
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
