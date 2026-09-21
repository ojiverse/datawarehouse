import { compareSnowflakes, type Snowflake } from "../../src/domain/ids";
import { EMPTY_RATE_LIMIT_HEADERS, type RateLimitHeaders } from "../../src/domain/rate-limit";
import type {
  BudgetDecision,
  BudgetGate,
  ChannelMessagesRequest,
  Clock,
  DiscordHttpResponse,
  DiscordMessagesClient,
  FaultInjector,
  FaultPoint,
  RequestReport,
} from "../../src/ports";
import { InjectedCrash } from "../../src/ports";

export function snowflake(value: number | bigint | string): Snowflake {
  return String(value) as Snowflake;
}

/**
 * Deterministic clock; tests advance it explicitly.
 *
 * The default epoch sits one year ahead of real time: workerd fires Durable Object alarms whose
 * scheduled time is already in the past, which would make alarm-driven tests race. Keeping every
 * fake timestamp in the future leaves `runDurableObjectAlarm` as the only trigger.
 */
export class FakeClock implements Clock {
  private counter = 0;
  constructor(private currentMs: number = Date.now() + 365 * 24 * 60 * 60 * 1_000) {}
  nowMs(): number {
    return this.currentMs;
  }
  advance(ms: number): void {
    this.currentMs += ms;
  }
  set(ms: number): void {
    this.currentMs = ms;
  }
  // Arrow property: callers pass this function around unbound (e.g. generateUuidV7).
  readonly randomBytes = (length: number): Uint8Array => {
    const bytes = new Uint8Array(length);
    for (let i = 0; i < length; i++) {
      this.counter = (this.counter + 1) % 251;
      bytes[i] = this.counter;
    }
    return bytes;
  };
}

export type ScriptedResponse =
  | {
      readonly kind: "status";
      readonly status: number;
      readonly body?: unknown;
      readonly headers?: Partial<RateLimitHeaders>;
    }
  | { readonly kind: "throw"; readonly message: string };

export type DiscordMessage = { readonly id: string; readonly content: string };

/**
 * In-memory Discord that serves Get Channel Messages newest→oldest and lets a test queue
 * scripted responses (429, 5xx, 401, 403, transport errors) ahead of the real pages.
 */
export class FakeDiscord implements DiscordMessagesClient {
  readonly requests: ChannelMessagesRequest[] = [];
  private readonly script: ScriptedResponse[] = [];
  private readonly messages: DiscordMessage[];
  private headers: Partial<RateLimitHeaders> = {};

  constructor(
    messages: readonly DiscordMessage[],
    private readonly clock: FakeClock,
  ) {
    this.messages = [...messages].sort(
      (a, b) => -compareSnowflakes(snowflake(a.id), snowflake(b.id)),
    );
  }

  enqueue(...responses: ScriptedResponse[]): void {
    this.script.push(...responses);
  }

  /** Rate-limit headers attached to every successful page from now on. */
  setSuccessHeaders(headers: Partial<RateLimitHeaders>): void {
    this.headers = headers;
  }

  pageFor(request: ChannelMessagesRequest): DiscordMessage[] {
    const eligible =
      request.before === null
        ? this.messages
        : this.messages.filter(
            (m) => compareSnowflakes(snowflake(m.id), request.before as Snowflake) < 0,
          );
    return eligible.slice(0, request.limit);
  }

  async fetchChannelMessages(request: ChannelMessagesRequest): Promise<DiscordHttpResponse> {
    this.requests.push(request);
    const startedAt = this.clock.nowMs();
    this.clock.advance(10);
    const completedAt = this.clock.nowMs();
    const scripted = this.script.shift();
    if (scripted?.kind === "throw") {
      throw new Error(scripted.message);
    }
    const status = scripted?.status ?? 200;
    const body = scripted ? (scripted.body ?? {}) : this.pageFor(request);
    const headers: RateLimitHeaders = {
      ...EMPTY_RATE_LIMIT_HEADERS,
      ...(scripted ? {} : this.headers),
      ...(scripted?.headers ?? {}),
    };
    return {
      status,
      rate_limit: headers,
      body: new TextEncoder().encode(JSON.stringify(body)),
      request_started_at: new Date(startedAt).toISOString(),
      response_completed_at: new Date(completedAt).toISOString(),
      response_completed_at_ms: completedAt,
    };
  }
}

/** Budget gate that always grants unless a test flips it. */
export class FakeBudget implements BudgetGate {
  readonly reports: RequestReport[] = [];
  decision: BudgetDecision = { granted: true };
  acquires = 0;

  async acquire(): Promise<BudgetDecision> {
    this.acquires += 1;
    return this.decision;
  }

  async report(report: RequestReport): Promise<void> {
    this.reports.push(report);
  }
}

/** Crashes once at the configured point, then behaves normally (like a restarted runtime). */
export class OneShotFault implements FaultInjector {
  private armed: FaultPoint | null;
  fired = 0;

  constructor(point: FaultPoint | null) {
    this.armed = point;
  }

  arm(point: FaultPoint): void {
    this.armed = point;
  }

  check(point: FaultPoint): void {
    if (this.armed === point) {
      this.armed = null;
      this.fired += 1;
      throw new InjectedCrash(point);
    }
  }
}

export function makeMessages(count: number, startId = 1_000_000): DiscordMessage[] {
  return Array.from({ length: count }, (_, i) => ({
    id: String(startId + i),
    content: `message ${i}`,
  }));
}
