import { cloudflareTest } from "@cloudflare/vitest-plugin";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [
    cloudflareTest({
      wrangler: { configPath: "./wrangler.jsonc" },
      miniflare: {
        bindings: {
          BACKFILL_API_TOKEN: "test-api-token",
          DISCORD_BOT_TOKEN: "test-bot-token",
          // Small budget so the exhaustion test needs ~90 RPC reports instead of ~9,000,
          // which exceeds the 5 s test timeout on CI runners.
          DISCORD_INVALID_REQUEST_BUDGET: "100",
        },
      },
    }),
  ],
});
