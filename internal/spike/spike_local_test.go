package spike_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/apache/iceberg-go"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/spike"
)

// TestClosedLoopAgainstLocalCatalog runs the full #36 loop against the docker
// stand-in in spike/local (MinIO + apache/iceberg-rest-fixture). It is the
// reference run for the Failure Rule: a failure that appears only on R2 Data
// Catalog is R2-specific, one that appears here is iceberg-go / protocol.
func TestClosedLoopAgainstLocalCatalog(t *testing.T) {
	if os.Getenv("DWH_SPIKE_LOCAL_CATALOG") != "1" {
		t.Skip("set DWH_SPIKE_LOCAL_CATALOG=1 with spike/local/docker-compose.yml running")
	}
	cfg := spike.Config{
		Archive: archive.Config{
			Endpoint: "http://localhost:9000", Region: "us-east-1", Bucket: "dwh-observations",
			AccessKeyID: "dwhspike", SecretAccessKey: "dwhspike-secret", UsePathStyle: true,
		},
		ArchivePrefix: "observations/v1/source=http_backfill/",
		Catalog: icebergcat.Config{
			URI: "http://localhost:8181", Warehouse: "s3://dwh-canonical/", Namespace: "dwh_spike_test",
			Props: iceberg.Properties{
				"s3.endpoint": "http://localhost:9000", "s3.region": "us-east-1",
				"s3.access-key-id": "dwhspike", "s3.secret-access-key": "dwhspike-secret",
				"s3.force-virtual-addressing": "false",
			},
		},
		TableName:      "message",
		Fixture:        observation.DefaultFixtureSpec(),
		DiscordEnvKeys: []string{"DISCORD_BOT_TOKEN"},
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Cleanup(func() { _ = spike.Cleanup(context.Background(), cfg, log) })
	rep, err := spike.Run(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if !rep.AllPassed() {
		t.Fatalf("criteria failed: %+v", rep.Criteria)
	}
	for _, c := range rep.Criteria {
		if c.Skipped && c.Name != "r2_sql_query" && c.Name != "r2_sql_query_after_rebuild" {
			t.Fatalf("unexpected skip %+v", c)
		}
	}
}
