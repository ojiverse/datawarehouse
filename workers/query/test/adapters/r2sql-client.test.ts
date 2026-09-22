import { afterEach, describe, expect, it, vi } from "vitest";
import { createR2SqlClient, R2SqlError } from "../../src/adapters/r2sql-client";

function stubFetch(status: number, body: string): { calls: Request[] } {
  const calls: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(new Request(input, init));
      return new Response(body, { status });
    }),
  );
  return { calls };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("R2 SQL client", () => {
  it("parses the wrapped Cloudflare v4 envelope shape (result.rows)", async () => {
    stubFetch(200, JSON.stringify({ success: true, errors: [], result: { rows: [{ n: 3 }] } }));
    const client = createR2SqlClient({ accountId: "acct", bucket: "bkt", token: "tok" });
    const rows = await client.query("SELECT 1");
    expect(rows).toEqual([{ n: 3 }]);
  });

  it("parses a bare JSON array and sends the expected request", async () => {
    const { calls } = stubFetch(200, JSON.stringify([{ message_id: "1" }]));
    const client = createR2SqlClient({ accountId: "acct", bucket: "bkt", token: "tok" });
    const rows = await client.query("SELECT message_id FROM ns.t");
    expect(rows).toEqual([{ message_id: "1" }]);

    expect(calls).toHaveLength(1);
    const [request] = calls;
    if (!request) throw new Error("fetch was not called");
    expect(request.headers.get("authorization")).toBe("Bearer tok");
    expect(new URL(request.url).pathname).toBe("/api/v1/accounts/acct/r2-sql/query/bkt");
    expect(await request.clone().json()).toEqual({ query: "SELECT message_id FROM ns.t" });
  });

  it("throws R2SqlError with the HTTP status on a non-2xx response", async () => {
    stubFetch(
      401,
      JSON.stringify({
        success: false,
        errors: [{ code: 10000, message: "Authentication error" }],
      }),
    );
    const client = createR2SqlClient({ accountId: "acct", bucket: "bkt", token: "tok" });
    await expect(client.query("SELECT 1")).rejects.toMatchObject({ httpStatus: 401 });
  });

  it("throws R2SqlError on an unrecognised response shape", async () => {
    stubFetch(200, JSON.stringify({ unexpected: true }));
    const client = createR2SqlClient({ accountId: "acct", bucket: "bkt", token: "tok" });
    await expect(client.query("SELECT 1")).rejects.toBeInstanceOf(R2SqlError);
  });

  it("respects a baseUrl override", async () => {
    const { calls } = stubFetch(200, JSON.stringify([]));
    const client = createR2SqlClient({
      accountId: "acct",
      bucket: "bkt",
      token: "tok",
      baseUrl: "https://example.test",
    });
    await client.query("SELECT 1");
    expect(calls[0]?.url).toBe("https://example.test/api/v1/accounts/acct/r2-sql/query/bkt");
  });
});
