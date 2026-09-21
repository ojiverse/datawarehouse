import { compareSnowflakes, minSnowflake, type Snowflake } from "./ids";
import type { BackfillRange } from "./run";

/**
 * Pagination rules for Get Channel Messages.
 *
 * Discord returns pages newest→oldest and forbids combining `before` with `after`, so the
 * crawl walks backwards with `before` only and treats `range.after` as a stop condition.
 * Message IDs are read from the page purely to derive the next cursor; the page body itself
 * is archived unmodified.
 */

export type CursorAdvance = {
  /** `before` for the next request, or `null` when the run is complete. */
  readonly next_before: Snowflake | null;
  readonly completed: boolean;
};

export function advanceCursor(
  messageIds: readonly Snowflake[],
  pageLimit: number,
  range: BackfillRange,
): CursorAdvance {
  const oldest = minSnowflake(messageIds);
  if (oldest === undefined) {
    return { next_before: null, completed: true };
  }
  if (range.after !== null && compareSnowflakes(oldest, range.after) <= 0) {
    return { next_before: null, completed: true };
  }
  // A short page means Discord had nothing older to return for this scope.
  if (messageIds.length < pageLimit) {
    return { next_before: null, completed: true };
  }
  return { next_before: oldest, completed: false };
}
