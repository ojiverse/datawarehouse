import { describe, expect, it } from "vitest";
import { advanceCursor } from "../../src/domain/cursor";
import { snowflake } from "../helpers/fakes";

const noRange = { after: null, before: null };

describe("advanceCursor", () => {
  it("continues from the oldest message of a full page", () => {
    const ids = [snowflake(30), snowflake(20), snowflake(10)];
    expect(advanceCursor(ids, 3, noRange)).toEqual({
      next_before: snowflake(10),
      completed: false,
    });
  });

  it("completes on an empty page", () => {
    expect(advanceCursor([], 100, noRange)).toEqual({ next_before: null, completed: true });
  });

  it("completes on a short page", () => {
    const ids = [snowflake(30), snowflake(20)];
    expect(advanceCursor(ids, 3, noRange)).toEqual({ next_before: null, completed: true });
  });

  it("completes when the page reaches the range lower bound", () => {
    const ids = [snowflake(30), snowflake(20), snowflake(10)];
    expect(advanceCursor(ids, 3, { after: snowflake(10), before: null })).toEqual({
      next_before: null,
      completed: true,
    });
    expect(advanceCursor(ids, 3, { after: snowflake(15), before: null })).toEqual({
      next_before: null,
      completed: true,
    });
  });

  it("keeps going while the page is above the lower bound", () => {
    const ids = [snowflake(30), snowflake(20), snowflake(10)];
    expect(advanceCursor(ids, 3, { after: snowflake(5), before: null })).toEqual({
      next_before: snowflake(10),
      completed: false,
    });
  });

  it("does not depend on Discord's ordering of the page", () => {
    const ids = [snowflake(10), snowflake(30), snowflake(20)];
    expect(advanceCursor(ids, 3, noRange).next_before).toBe(snowflake(10));
  });
});
