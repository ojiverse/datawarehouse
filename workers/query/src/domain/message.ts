/**
 * Canonical Message domain fields (docs/domain/canonical-store/README.md,
 * internal/canonical/schema.go) as exposed by the Query API. Field names
 * match the Iceberg column names exactly (the repo's convention for wire
 * shapes that mirror a persisted contract 1:1, e.g. workers/backfill's
 * `StartRunRequest`), so a caller can verify a query result against the
 * Canonical schema without a name-mapping step. This Worker never invents a
 * field the Canonical Message model does not define.
 *
 * Snowflakes (`message_id`, `channel_id`, `guild_id`, `author_id`) stay
 * decimal strings end to end, matching docs/domain/observations/identity.md:
 * a JavaScript number cannot hold a 64-bit Snowflake without loss.
 */
export type CanonicalMessage = {
  readonly message_id: string;
  readonly channel_id: string;
  readonly guild_id: string;
  readonly author_id: string;
  readonly author_username: string | null;
  readonly author_is_bot: boolean;
  readonly content: string | null;
  /** Discord-side message creation time (authoritative), ISO 8601 UTC. */
  readonly created_at: string;
  /** Discord-side edit time, or null when the adopted snapshot is unedited. */
  readonly edited_at: string | null;
  readonly pinned: boolean;
  readonly message_type: number;
  /** Observation ID the current best-known snapshot was adopted from (traceability). */
  readonly observation_id: string;
  readonly observed_at: string;
  readonly source_kind: string;
  readonly projection_version: string;
};

/** One row as R2 SQL returns it: JSON scalars only, beta/undocumented typing. */
export type R2SqlRow = Readonly<Record<string, unknown>>;

/** Thrown when an R2 SQL row is missing a column the Canonical schema requires. */
export class MalformedRowError extends Error {
  constructor(column: string, row: R2SqlRow) {
    super(`r2 sql row missing or malformed column "${column}": ${JSON.stringify(row)}`);
    this.name = "MalformedRowError";
  }
}

function readString(row: R2SqlRow, column: string): string {
  const value = row[column];
  if (typeof value === "string") return value;
  if (typeof value === "number") return String(value);
  throw new MalformedRowError(column, row);
}

function readOptionalString(row: R2SqlRow, column: string): string | null {
  const value = row[column];
  if (value === null || value === undefined) return null;
  if (typeof value === "string") return value;
  if (typeof value === "number") return String(value);
  throw new MalformedRowError(column, row);
}

function readBoolean(row: R2SqlRow, column: string): boolean {
  const value = row[column];
  if (typeof value === "boolean") return value;
  if (value === "true" || value === "false") return value === "true";
  throw new MalformedRowError(column, row);
}

function readInt(row: R2SqlRow, column: string): number {
  const value = row[column];
  if (typeof value === "number" && Number.isInteger(value)) return value;
  if (typeof value === "string" && /^-?[0-9]+$/.test(value)) return Number.parseInt(value, 10);
  throw new MalformedRowError(column, row);
}

/**
 * Maps one raw R2 SQL `message` table row into the Canonical Message domain
 * shape. This is the single place that translates R2 SQL's beta/undocumented
 * JSON typing (docs/architecture/cloudflare/processing/iceberg-spike-result.md)
 * into the stable domain model callers see.
 *
 * @throws {MalformedRowError} a required column is absent or has an
 *   unexpected JSON type.
 */
export function mapRowToCanonicalMessage(row: R2SqlRow): CanonicalMessage {
  return {
    message_id: readString(row, "message_id"),
    channel_id: readString(row, "channel_id"),
    guild_id: readString(row, "guild_id"),
    author_id: readString(row, "author_id"),
    author_username: readOptionalString(row, "author_username"),
    author_is_bot: readBoolean(row, "author_is_bot"),
    content: readOptionalString(row, "content"),
    created_at: readString(row, "created_at"),
    edited_at: readOptionalString(row, "edited_at"),
    pinned: readBoolean(row, "pinned"),
    message_type: readInt(row, "message_type"),
    observation_id: readString(row, "observation_id"),
    observed_at: readString(row, "observed_at"),
    source_kind: readString(row, "source_kind"),
    projection_version: readString(row, "projection_version"),
  };
}

/** One row of the per-Channel Message count aggregate. */
export type ChannelMessageCount = {
  readonly channel_id: string;
  readonly message_count: number;
};

/**
 * Maps one raw R2 SQL `GROUP BY channel_id` aggregate row.
 *
 * @throws {MalformedRowError} `channel_id` or `message_count` is absent or
 *   has an unexpected JSON type.
 */
export function mapRowToChannelMessageCount(row: R2SqlRow): ChannelMessageCount {
  return {
    channel_id: readString(row, "channel_id"),
    message_count: readInt(row, "message_count"),
  };
}
