import { cloudflareTest } from "@cloudflare/vitest-plugin";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [
    cloudflareTest({
      wrangler: { configPath: "./wrangler.jsonc" },
      miniflare: {
        bindings: {
          QUERY_API_TOKEN: "test-api-token",
          R2_SQL_TOKEN: "test-r2sql-token",
          R2_SQL_ACCOUNT_ID: "test-account",
          R2_SQL_BUCKET: "test-bucket",
          R2_SQL_NAMESPACE: "dwh_test",
          R2_SQL_TABLE: "message",
        },
      },
    }),
  ],
});
