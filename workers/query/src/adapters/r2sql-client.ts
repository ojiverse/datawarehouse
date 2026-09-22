import type { R2SqlRow } from "../domain/message";

/** Configuration to reach the Cloudflare R2 SQL HTTP API for one warehouse bucket. */
export type R2SqlConfig = {
  readonly accountId: string;
  readonly bucket: string;
  readonly token: string;
  /** Defaults to the public R2 SQL endpoint; overridable for tests. */
  readonly baseUrl?: string;
};

const DEFAULT_BASE_URL = "https://api.sql.cloudflarestorage.com";

/**
 * Executes read-only SQL against Canonical Store Iceberg tables via R2 SQL.
 * This is the only I/O boundary between this Worker and the Canonical Store;
 * it never reads Observation Archive raw objects (docs/architecture/cloudflare/canonical-store).
 */
export interface R2SqlPort {
  /**
   * Runs one SQL statement and returns its rows.
   *
   * @throws {R2SqlError} the HTTP call failed, R2 SQL reported a query error,
   *   or the response body shape could not be recognised (R2 SQL is beta and
   *   its response shape is not fully documented upstream).
   */
  query(sql: string): Promise<readonly R2SqlRow[]>;
}

/** Raised when an R2 SQL call fails, wrapping the HTTP status and a response excerpt. */
export class R2SqlError extends Error {
  readonly httpStatus: number | undefined;

  constructor(message: string, httpStatus?: number) {
    super(message);
    this.name = "R2SqlError";
    this.httpStatus = httpStatus;
  }
}

type Envelope = {
  readonly success?: boolean;
  readonly errors?: readonly unknown[];
  readonly result?: unknown;
};

function truncate(text: string): string {
  return text.length > 512 ? `${text.slice(0, 512)}...` : text;
}

function rowsFromResult(result: unknown): readonly R2SqlRow[] {
  if (Array.isArray(result)) return result as R2SqlRow[];
  if (result !== null && typeof result === "object") {
    const obj = result as { rows?: unknown; results?: unknown };
    if (Array.isArray(obj.rows)) return obj.rows as R2SqlRow[];
    if (Array.isArray(obj.results)) return obj.results as R2SqlRow[];
  }
  throw new R2SqlError(`r2 sql: unrecognised result shape: ${truncate(JSON.stringify(result))}`);
}

function extractRows(raw: string): readonly R2SqlRow[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new R2SqlError(`r2 sql: response was not JSON: ${truncate(raw)}`);
  }
  if (parsed !== null && typeof parsed === "object" && "result" in parsed) {
    const envelope = parsed as Envelope;
    if (envelope.errors && envelope.errors.length > 0 && envelope.success === false) {
      throw new R2SqlError(`r2 sql error: ${truncate(raw)}`);
    }
    return rowsFromResult(envelope.result);
  }
  return rowsFromResult(parsed);
}

/** Builds the production R2 SQL client backed by `fetch`. */
export function createR2SqlClient(config: R2SqlConfig): R2SqlPort {
  const baseUrl = config.baseUrl ?? DEFAULT_BASE_URL;
  const endpoint = `${baseUrl}/api/v1/accounts/${config.accountId}/r2-sql/query/${config.bucket}`;
  return {
    async query(sql: string): Promise<readonly R2SqlRow[]> {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: {
          authorization: `Bearer ${config.token}`,
          "content-type": "application/json",
        },
        body: JSON.stringify({ query: sql }),
      });
      const raw = await response.text();
      if (!response.ok) {
        throw new R2SqlError(`r2 sql http ${response.status}: ${truncate(raw)}`, response.status);
      }
      return extractRows(raw);
    },
  };
}
