import { env } from "cloudflare:test";

/**
 * Removes every object from the test Observation bucket.
 *
 * The Cloudflare Vitest plugin shares the miniflare R2 bucket across tests in a file, so each
 * test that asserts on object counts starts from an empty bucket explicitly.
 */
export async function purgeObservations(): Promise<void> {
  let cursor: string | undefined;
  do {
    const page = await env.OBSERVATIONS.list(cursor === undefined ? {} : { cursor });
    if (page.objects.length > 0) {
      await env.OBSERVATIONS.delete(page.objects.map((o) => o.key));
    }
    cursor = page.truncated ? page.cursor : undefined;
  } while (cursor !== undefined);
}
