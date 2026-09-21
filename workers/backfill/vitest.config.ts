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
        },
      },
    }),
  ],
});
