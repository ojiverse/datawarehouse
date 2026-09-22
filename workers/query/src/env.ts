/** Bindings declared in wrangler.jsonc. Secrets are injected with `wrangler secret put`. */
export type Env = {
  readonly ENVIRONMENT: string;
  /** Cloudflare account ID that owns the R2 SQL warehouse (not secret; also recorded in docs). */
  readonly R2_SQL_ACCOUNT_ID: string;
  /** R2 bucket name backing the Canonical Store warehouse R2 SQL queries against. */
  readonly R2_SQL_BUCKET: string;
  /** Iceberg REST Catalog namespace the Canonical Message table lives in. */
  readonly R2_SQL_NAMESPACE: string;
  /** Canonical Message table name (`internal/canonical.TableName` on the Go side). */
  readonly R2_SQL_TABLE: string;
  /** Optional override of the R2 SQL HTTP API base URL, for tests only. */
  readonly R2_SQL_BASE_URL?: string;
  readonly QUERY_DEFAULT_PAGE_LIMIT: string;
  readonly QUERY_MAX_PAGE_LIMIT: string;
  /** Bearer token an R2 SQL query is authenticated with (Cloudflare API token, R2 SQL scope). */
  readonly R2_SQL_TOKEN: string;
  /** Bearer token this Worker's own API requires from its caller. */
  readonly QUERY_API_TOKEN: string;
};

/** Parses a Worker var as a positive integer, falling back when absent or invalid. */
export function readPositiveInt(value: string | undefined, fallback: number): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}
