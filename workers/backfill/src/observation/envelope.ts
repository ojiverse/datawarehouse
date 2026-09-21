import type { Snowflake, UuidV7 } from "../domain/ids";
import type { RateLimitHeaders } from "../domain/rate-limit";

/**
 * Observation Envelope v1 for HTTP Backfill (docs/domain/observations/envelope.md).
 *
 * JSON field names are snake_case because this object is a cross-language contract shared
 * with the Go replay reader (#38); the fixture under contracts/observation-envelope/v1 is
 * the compatibility oracle for both implementations.
 */

export const ENVELOPE_VERSION = "v1" as const;
export const SOURCE_KIND_HTTP_BACKFILL = "http_backfill" as const;
export const OPERATION_GET_CHANNEL_MESSAGES = "get_channel_messages" as const;

export type HttpPagination = {
  readonly before: Snowflake | null;
  readonly after: Snowflake | null;
};

export type HttpCapabilities = {
  /** Whether the application has the Message Content privileged intent enabled. */
  readonly message_content: boolean;
};

export type HttpRateLimitProvenance = {
  readonly limit: number | null;
  readonly remaining: number | null;
  readonly reset_after_seconds: number | null;
  readonly bucket: string | null;
};

export type HttpBackfillProvenanceV1 = {
  readonly run_id: UuidV7;
  readonly discord_api_version: string;
  readonly guild_id: Snowflake;
  readonly channel_id: Snowflake;
  readonly operation: typeof OPERATION_GET_CHANNEL_MESSAGES;
  readonly endpoint: string;
  readonly pagination: HttpPagination;
  readonly limit: number;
  readonly request_started_at: string;
  readonly response_completed_at: string;
  readonly http_status: number;
  readonly capabilities: HttpCapabilities;
  readonly rate_limit: HttpRateLimitProvenance;
};

export type HttpBackfillEnvelopeV1 = {
  readonly envelope_version: typeof ENVELOPE_VERSION;
  readonly observation_id: UuidV7;
  readonly source_kind: typeof SOURCE_KIND_HTTP_BACKFILL;
  readonly observed_at: string;
  /** Entire response body of the page, parsed as JSON and stored without field selection. */
  readonly payload: unknown;
  readonly provenance: HttpBackfillProvenanceV1;
};

export type BuildEnvelopeInput = {
  readonly observation_id: UuidV7;
  readonly run_id: UuidV7;
  readonly discord_api_version: string;
  readonly guild_id: Snowflake;
  readonly channel_id: Snowflake;
  readonly pagination: HttpPagination;
  readonly limit: number;
  readonly request_started_at: string;
  readonly response_completed_at: string;
  readonly http_status: number;
  readonly payload: unknown;
  readonly capabilities: HttpCapabilities;
  readonly rate_limit: RateLimitHeaders;
};

export function channelMessagesEndpoint(channelId: Snowflake): string {
  return `/channels/${channelId}/messages`;
}

export function buildHttpBackfillEnvelope(input: BuildEnvelopeInput): HttpBackfillEnvelopeV1 {
  return {
    envelope_version: ENVELOPE_VERSION,
    observation_id: input.observation_id,
    source_kind: SOURCE_KIND_HTTP_BACKFILL,
    // Observed At is the moment the producer finished receiving the payload (envelope.md).
    observed_at: input.response_completed_at,
    payload: input.payload,
    provenance: {
      run_id: input.run_id,
      discord_api_version: input.discord_api_version,
      guild_id: input.guild_id,
      channel_id: input.channel_id,
      operation: OPERATION_GET_CHANNEL_MESSAGES,
      endpoint: channelMessagesEndpoint(input.channel_id),
      pagination: input.pagination,
      limit: input.limit,
      request_started_at: input.request_started_at,
      response_completed_at: input.response_completed_at,
      http_status: input.http_status,
      capabilities: input.capabilities,
      rate_limit: {
        limit: input.rate_limit.limit,
        remaining: input.rate_limit.remaining,
        reset_after_seconds: input.rate_limit.reset_after_seconds,
        bucket: input.rate_limit.bucket,
      },
    },
  };
}
