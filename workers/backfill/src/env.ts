import type { BackfillChannelDurableObject } from "./do/backfill-channel";
import type { DiscordHttpBudgetDurableObject } from "./do/discord-http-budget";

/** Bindings declared in wrangler.jsonc. Secrets are injected with `wrangler secret put`. */
export type Env = {
  readonly ENVIRONMENT: string;
  readonly DISCORD_API_BASE_URL: string;
  readonly DISCORD_API_VERSION: string;
  readonly DISCORD_MESSAGE_CONTENT_ENABLED: string;
  readonly BACKFILL_PAGES_PER_ALARM: string;
  readonly BACKFILL_PAGE_LIMIT: string;
  readonly DISCORD_GLOBAL_REQUESTS_PER_SECOND: string;
  readonly DISCORD_INVALID_REQUEST_BUDGET: string;
  readonly DISCORD_INVALID_REQUEST_WINDOW_SECONDS: string;
  readonly DISCORD_BOT_TOKEN: string;
  readonly BACKFILL_API_TOKEN: string;
  readonly OBSERVATIONS: R2Bucket;
  readonly BACKFILL_CHANNEL: DurableObjectNamespace<BackfillChannelDurableObject>;
  readonly DISCORD_HTTP_BUDGET: DurableObjectNamespace<DiscordHttpBudgetDurableObject>;
};

export function readPositiveInt(value: string | undefined, fallback: number): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

export function readBoolean(value: string | undefined, fallback: boolean): boolean {
  if (value === "true") return true;
  if (value === "false") return false;
  return fallback;
}

/** Durable Object name for the Channel owner: stable per environment and Channel ID. */
export function channelObjectName(environment: string, channelId: string): string {
  return `${environment}:channel:${channelId}`;
}

/** Durable Object name for the application-wide budget owner. */
export function budgetObjectName(environment: string): string {
  return `${environment}:discord-http-budget`;
}
