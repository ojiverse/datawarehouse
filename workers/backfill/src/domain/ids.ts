/**
 * Identity primitives shared by the Backfill control plane.
 *
 * Discord Snowflakes are kept as decimal strings end-to-end (docs/domain/observations/identity.md):
 * a JavaScript number cannot hold a 64-bit Snowflake without loss, so comparisons use BigInt.
 */

/** A Discord Snowflake in its canonical decimal-string representation. */
export type Snowflake = string & { readonly __brand: "Snowflake" };

/** RFC 9562 UUIDv7 used for Observation and Run identity. */
export type UuidV7 = string & { readonly __brand: "UuidV7" };

const SNOWFLAKE_PATTERN = /^[0-9]{1,20}$/;
const UUID_V7_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

/**
 * Validates an untrusted string as a Snowflake.
 * Returns `undefined` for anything that is not a decimal digit string.
 */
export function parseSnowflake(value: unknown): Snowflake | undefined {
  if (typeof value !== "string" || !SNOWFLAKE_PATTERN.test(value)) {
    return undefined;
  }
  return value as Snowflake;
}

/** Compares two Snowflakes numerically without precision loss. */
export function compareSnowflakes(a: Snowflake, b: Snowflake): -1 | 0 | 1 {
  const left = BigInt(a);
  const right = BigInt(b);
  if (left < right) return -1;
  if (left > right) return 1;
  return 0;
}

/** Returns the numerically smallest Snowflake of a non-empty list. */
export function minSnowflake(values: readonly Snowflake[]): Snowflake | undefined {
  let min: Snowflake | undefined;
  for (const value of values) {
    if (min === undefined || compareSnowflakes(value, min) < 0) {
      min = value;
    }
  }
  return min;
}

/** Validates an untrusted string as a UUIDv7 (lower-case canonical form). */
export function parseUuidV7(value: unknown): UuidV7 | undefined {
  if (typeof value !== "string" || !UUID_V7_PATTERN.test(value)) {
    return undefined;
  }
  return value as UuidV7;
}

/**
 * Generates a UUIDv7 from the supplied Unix-millisecond timestamp and a random source.
 *
 * The monotonic counter of RFC 9562 is deliberately not implemented: identity.md forbids
 * relying on UUIDv7 ordering across producers, so 74 random bits are sufficient here.
 */
export function generateUuidV7(nowMs: number, randomBytes: (length: number) => Uint8Array): UuidV7 {
  const bytes = new Uint8Array(16);
  const ts = BigInt(Math.max(0, Math.floor(nowMs)));
  bytes[0] = Number((ts >> 40n) & 0xffn);
  bytes[1] = Number((ts >> 32n) & 0xffn);
  bytes[2] = Number((ts >> 24n) & 0xffn);
  bytes[3] = Number((ts >> 16n) & 0xffn);
  bytes[4] = Number((ts >> 8n) & 0xffn);
  bytes[5] = Number(ts & 0xffn);
  const random = randomBytes(10);
  bytes.set(random, 6);
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x70;
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}` as UuidV7;
}
