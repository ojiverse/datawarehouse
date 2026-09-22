// Package icebergcat wires iceberg-go to an Iceberg REST Catalog (R2 Data
// Catalog in production, apache/iceberg-rest-fixture locally) and implements the
// commit invariants required by docs/architecture/cloudflare/processing/materialization.md.
package icebergcat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	icebergio "github.com/apache/iceberg-go/io"
	// Registers the s3:// FileIO scheme; without this side-effect import
	// iceberg-go cannot read metadata or write data files on R2 / MinIO.
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"

	"github.com/ojiverse/datawarehouse/internal/canonical"
)

// Config describes the REST catalog connection.
type Config struct {
	URI       string
	Warehouse string
	// Token is the bearer token (R2 API token). Empty for unauthenticated local catalogs.
	Token     string
	Namespace string
	// Props are extra catalog / FileIO properties (s3.endpoint, s3.region,
	// static s3 keys for catalogs that do not vend credentials).
	Props iceberg.Properties
}

// Client owns one catalog connection and one namespace.
type Client struct {
	cat *rest.Catalog
	ns  table.Identifier
	log *slog.Logger
}

// New connects to the catalog. iceberg-go requests vended credentials
// automatically (X-Iceberg-Access-Delegation), so with R2 the bearer token is
// the only secret needed for data file access.
func New(ctx context.Context, cfg Config, log *slog.Logger) (*Client, error) {
	if cfg.URI == "" || cfg.Namespace == "" {
		return nil, errors.New("icebergcat: uri and namespace are required")
	}
	opts := []rest.Option{rest.WithWarehouseLocation(cfg.Warehouse)}
	if cfg.Token != "" {
		opts = append(opts, rest.WithOAuthToken(cfg.Token))
	}
	if len(cfg.Props) > 0 {
		opts = append(opts, rest.WithAdditionalProps(cfg.Props))
	}
	cat, err := rest.NewCatalog(ctx, "dwh", cfg.URI, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect rest catalog: %w", err)
	}
	return &Client{cat: cat, ns: table.Identifier{cfg.Namespace}, log: log}, nil
}

// EnsureNamespace creates the namespace when missing.
func (c *Client) EnsureNamespace(ctx context.Context) error {
	exists, err := c.cat.CheckNamespaceExists(ctx, c.ns)
	if err != nil {
		return fmt.Errorf("check namespace: %w", err)
	}
	if exists {
		return nil
	}
	if err := c.cat.CreateNamespace(ctx, c.ns, nil); err != nil && !errors.Is(err, catalog.ErrNamespaceAlreadyExists) {
		return fmt.Errorf("create namespace: %w", err)
	}
	return nil
}

func (c *Client) ident(name string) table.Identifier {
	return append(append(table.Identifier{}, c.ns...), name)
}

// EnsureTable loads the Canonical table, creating it with the fixed schema and
// Zstandard Parquet compression when it does not exist.
func (c *Client) EnsureTable(ctx context.Context, name string) (*table.Table, error) {
	id := c.ident(name)
	tbl, err := c.cat.LoadTable(ctx, id)
	if err == nil {
		return tbl, nil
	}
	if !errors.Is(err, catalog.ErrNoSuchTable) {
		return nil, fmt.Errorf("load table: %w", err)
	}
	tbl, err = c.cat.CreateTable(ctx, id, canonical.MessageSchema(), catalog.WithProperties(iceberg.Properties{
		table.ParquetCompressionKey: "zstd",
	}))
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	c.log.Info("table created", slog.String("table", strings.Join(id, ".")), slog.String("location", tbl.Location()))
	return tbl, nil
}

// LoadTable re-reads the table from the catalog (used after a simulated crash).
func (c *Client) LoadTable(ctx context.Context, name string) (*table.Table, error) {
	tbl, err := c.cat.LoadTable(ctx, c.ident(name))
	if err != nil {
		return nil, fmt.Errorf("load table: %w", err)
	}
	return tbl, nil
}

// WriteStaging writes bytes to path through the table's FileIO (so vended
// credentials are used). The write is unconditional: staging identity is
// deterministic, so re-writing the same chunk yields the same content.
func (c *Client) WriteStaging(ctx context.Context, tbl *table.Table, path string, data []byte) error {
	fs, err := tbl.FS(ctx)
	if err != nil {
		return fmt.Errorf("table fs: %w", err)
	}
	wfs, ok := fs.(icebergio.WriteFileIO)
	if !ok {
		return fmt.Errorf("table fs %T does not support writes", fs)
	}
	if err := wfs.WriteFile(path, data); err != nil {
		return fmt.Errorf("write staging %s: %w", path, err)
	}
	return nil
}

// CommitStatus is the outcome of CommitFiles.
type CommitStatus string

// Outcomes of CommitFiles.
const (
	Committed         CommitStatus = "committed"
	AlreadyReferenced CommitStatus = "already_referenced"
)

// CommitResult carries the status and the resulting snapshot id.
type CommitResult struct {
	Status     CommitStatus
	SnapshotID int64
	DataFiles  int
}

// CommitFiles registers existing Parquet files with duplicate protection.
// When every path is already referenced by the current snapshot (a retry after
// a crash between commit and checkpoint), it returns AlreadyReferenced instead
// of creating a second snapshot; a partial overlap is an error because the
// staging identity contract says a chunk is committed atomically.
func (c *Client) CommitFiles(ctx context.Context, tbl *table.Table, paths []string) (CommitResult, error) {
	referenced, err := c.ReferencedFiles(ctx, tbl)
	if err != nil {
		return CommitResult{}, err
	}
	already := 0
	for _, p := range paths {
		if _, ok := referenced[p]; ok {
			already++
		}
	}
	switch {
	case already == len(paths):
		return CommitResult{Status: AlreadyReferenced, SnapshotID: snapshotID(tbl), DataFiles: len(referenced)}, nil
	case already > 0:
		return CommitResult{}, fmt.Errorf("commit: %d of %d files already referenced; chunk is not atomic", already, len(paths))
	}
	tx := tbl.NewTransaction()
	if err := tx.AddFiles(ctx, paths, nil, false); err != nil {
		return CommitResult{}, fmt.Errorf("add files: %w", err)
	}
	newTbl, err := tx.Commit(ctx)
	if err != nil {
		return CommitResult{}, fmt.Errorf("commit: %w", err)
	}
	*tbl = *newTbl
	referenced, err = c.ReferencedFiles(ctx, tbl)
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Status: Committed, SnapshotID: snapshotID(tbl), DataFiles: len(referenced)}, nil
}

// AddFilesRaw calls iceberg-go AddFiles without the pre-check, to observe the
// library's own duplicate rejection ("cannot add files that are already referenced").
func (c *Client) AddFilesRaw(ctx context.Context, tbl *table.Table, paths []string) error {
	tx := tbl.NewTransaction()
	if err := tx.AddFiles(ctx, paths, nil, false); err != nil {
		return err
	}
	_, err := tx.Commit(ctx)
	return err
}

// ReferencedFiles lists data file paths referenced by the current snapshot.
func (c *Client) ReferencedFiles(ctx context.Context, tbl *table.Table) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	snap := tbl.CurrentSnapshot()
	if snap == nil {
		return out, nil
	}
	fs, err := tbl.FS(ctx)
	if err != nil {
		return nil, fmt.Errorf("table fs: %w", err)
	}
	manifests, err := snap.Manifests(fs)
	if err != nil {
		return nil, fmt.Errorf("read manifests: %w", err)
	}
	for _, m := range manifests {
		entries, err := m.FetchEntries(fs, false)
		if err != nil {
			return nil, fmt.Errorf("read manifest entries: %w", err)
		}
		for _, e := range entries {
			if e.Status() != iceberg.EntryStatusDELETED {
				out[e.DataFile().FilePath()] = struct{}{}
			}
		}
	}
	return out, nil
}

func snapshotID(tbl *table.Table) int64 {
	if s := tbl.CurrentSnapshot(); s != nil {
		return s.SnapshotID
	}
	return 0
}

// DropTable removes the table from the catalog. PurgeTable is preferred so the
// data files go too; catalogs that reject purge fall back to a plain drop.
func (c *Client) DropTable(ctx context.Context, name string) error {
	id := c.ident(name)
	if err := c.cat.PurgeTable(ctx, id); err == nil {
		return nil
	} else {
		c.log.Warn("purge rejected, falling back to drop", slog.String("error", err.Error()))
	}
	if err := c.cat.DropTable(ctx, id); err != nil && !errors.Is(err, catalog.ErrNoSuchTable) {
		return fmt.Errorf("drop table: %w", err)
	}
	return nil
}

// TableExists reports whether the table is present in the catalog.
func (c *Client) TableExists(ctx context.Context, name string) (bool, error) {
	return c.cat.CheckTableExists(ctx, c.ident(name))
}
