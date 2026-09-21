import { describe, expect, it } from "vitest";
import {
  compareSnowflakes,
  generateUuidV7,
  minSnowflake,
  parseSnowflake,
  parseUuidV7,
} from "../../src/domain/ids";
import { snowflake } from "../helpers/fakes";

describe("Snowflake", () => {
  it("accepts decimal strings and rejects everything else", () => {
    expect(parseSnowflake("1234567890123456789")).toBe("1234567890123456789");
    expect(parseSnowflake(1234)).toBeUndefined();
    expect(parseSnowflake("12ab")).toBeUndefined();
    expect(parseSnowflake("")).toBeUndefined();
  });

  it("compares beyond 2^53 without precision loss", () => {
    const a = snowflake("9007199254740993");
    const b = snowflake("9007199254740992");
    expect(compareSnowflakes(a, b)).toBe(1);
    expect(compareSnowflakes(b, a)).toBe(-1);
    expect(compareSnowflakes(a, a)).toBe(0);
    expect(minSnowflake([a, b])).toBe(b);
    expect(minSnowflake([])).toBeUndefined();
  });
});

describe("UUIDv7", () => {
  it("generates RFC 9562 version 7 identifiers with the timestamp prefix", () => {
    const id = generateUuidV7(0x018f_0000_0000, () => new Uint8Array(10).fill(0xff));
    expect(parseUuidV7(id)).toBe(id);
    expect(id.startsWith("018f0000-0000-7")).toBe(true);
    expect(id.charAt(19)).toMatch(/[89ab]/);
  });

  it("rejects non-v7 UUIDs", () => {
    expect(parseUuidV7("123e4567-e89b-12d3-a456-426614174000")).toBeUndefined();
  });
});
