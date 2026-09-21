import { parseRateLimitHeaders } from "../domain/rate-limit";
import type { ChannelMessagesRequest, DiscordHttpResponse, DiscordMessagesClient } from "../ports";

export type DiscordClientConfig = {
  readonly base_url: string;
  readonly bot_token: string;
  readonly user_agent: string;
};

/** fetch-based Get Channel Messages client. The token never leaves this adapter. */
export class FetchDiscordMessagesClient implements DiscordMessagesClient {
  constructor(private readonly config: DiscordClientConfig) {}

  async fetchChannelMessages(request: ChannelMessagesRequest): Promise<DiscordHttpResponse> {
    const url = new URL(`${this.config.base_url}/channels/${request.channel_id}/messages`);
    url.searchParams.set("limit", String(request.limit));
    if (request.before !== null) {
      url.searchParams.set("before", request.before);
    }
    const startedAt = new Date();
    const response = await fetch(url, {
      method: "GET",
      headers: {
        Authorization: `Bot ${this.config.bot_token}`,
        "User-Agent": this.config.user_agent,
        Accept: "application/json",
      },
    });
    const body = new Uint8Array(await response.arrayBuffer());
    const completedAt = new Date();
    return {
      status: response.status,
      rate_limit: parseRateLimitHeaders(response.headers),
      body,
      request_started_at: startedAt.toISOString(),
      response_completed_at: completedAt.toISOString(),
      response_completed_at_ms: completedAt.getTime(),
    };
  }
}
