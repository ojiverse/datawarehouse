/**
 * Query building for the Canonical Message table, over the R2 SQL HTTP API.
 *
 * R2 SQL has no prepared-statement / bind-parameter API (only a raw `query`
 * string field, see internal/r2sql and
 * docs/architecture/cloudflare/processing/iceberg-spike-result.md), so every
 * value interpolated into SQL text here is validated first: Snowflakes must
 * match `SNOWFLAKE_PATTERN` and timestamps are re-serialized from a parsed
 * `Date`, never passed through verbatim.
 */

/** A Discord Snowflake in its canonical decimal-string representation. */
export type Snowflake = string & { readonly __brand: "Snowflake" };

const SNOWFLAKE_PATTERN = /^[0-9]{1,20}$/;

/** Validates an untrusted string as a Snowflake. Returns `undefined` otherwise. */
export function parseSnowflake(value: unknown): Snowflake | undefined {
  if (typeof value !== "string" || !SNOWFLAKE_PATTERN.test(value)) return undefined;
  return value as Snowflake;
}

/**
 * Validates an untrusted string as an instant in time.
 * Returns `undefined` for anything `Date` cannot parse.
 */
export function parseTimestamp(value: unknown): Date | undefined {
  if (typeof value !== "string" || value === "") return undefined;
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed;
}

/** The Canonical Message table, qualified with its Iceberg REST Catalog namespace. */
export type TableRef = {
  readonly namespace: string;
  readonly table: string;
};

function qualifiedName(ref: TableRef): string {
  return `${ref.namespace}.${ref.table}`;
}

/** R2 SQL accepted `TIMESTAMP '<ISO 8601 UTC>'` literals in real-environment testing. */
function timestampLiteral(instant: Date): string {
  return `TIMESTAMP '${instant.toISOString()}'`;
}

function stringLiteral(value: Snowflake): string {
  return `'${value}'`;
}

const MESSAGE_COLUMNS =
  "message_id, channel_id, guild_id, author_id, author_username, author_is_bot, content, " +
  "created_at, edited_at, pinned, message_type, observation_id, observed_at, source_kind, " +
  "projection_version";

/** A Discord-side created-at range filter. Either bound may be omitted. */
export type CreatedAtRange = {
  readonly after?: Date;
  readonly before?: Date;
};

/** Keyset pagination: only rows with `message_id` greater than the cursor. */
export type Page = {
  readonly limit: number;
  readonly afterMessageId?: Snowflake;
};

function whereCreatedAtRange(range: CreatedAtRange | undefined): readonly string[] {
  if (!range) return [];
  const clauses: string[] = [];
  if (range.after) clauses.push(`created_at >= ${timestampLiteral(range.after)}`);
  if (range.before) clauses.push(`created_at < ${timestampLiteral(range.before)}`);
  return clauses;
}

/** Builds `SELECT ... WHERE message_id = ?` (Query 1: get a Message by ID). */
export function buildGetMessageByIdQuery(table: TableRef, messageId: Snowflake): string {
  return `SELECT ${MESSAGE_COLUMNS} FROM ${qualifiedName(table)} WHERE message_id = ${stringLiteral(messageId)}`;
}

/**
 * Builds a list query scoped to one equality column (`channel_id` or
 * `author_id`), an optional Discord-side created-at range, and keyset
 * pagination ordered by `message_id`.
 *
 * Used for Query 2 (List Messages by Channel), Query 3 (List Messages by
 * Author) and Query 4 (created-at range filter, composed with either).
 */
export function buildListMessagesQuery(
  table: TableRef,
  scopeColumn: "channel_id" | "author_id",
  scopeValue: Snowflake,
  range: CreatedAtRange | undefined,
  page: Page,
): string {
  const clauses = [`${scopeColumn} = ${stringLiteral(scopeValue)}`, ...whereCreatedAtRange(range)];
  if (page.afterMessageId) {
    clauses.push(`message_id > ${stringLiteral(page.afterMessageId)}`);
  }
  const where = clauses.join(" AND ");
  return (
    `SELECT ${MESSAGE_COLUMNS} FROM ${qualifiedName(table)} WHERE ${where} ` +
    `ORDER BY message_id LIMIT ${page.limit}`
  );
}

/** Builds `SELECT channel_id, COUNT(*) ... GROUP BY channel_id` (Query 5: per-Channel aggregate). */
export function buildChannelMessageCountQuery(table: TableRef, limit: number): string {
  return (
    `SELECT channel_id, COUNT(*) AS message_count FROM ${qualifiedName(table)} ` +
    `GROUP BY channel_id ORDER BY channel_id LIMIT ${limit}`
  );
}
