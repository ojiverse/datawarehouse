import { createR2SqlClient } from "./adapters/r2sql-client";
import type { Env } from "./env";
import { route } from "./worker/api";

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const sql = createR2SqlClient({
      accountId: env.R2_SQL_ACCOUNT_ID,
      bucket: env.R2_SQL_BUCKET,
      token: env.R2_SQL_TOKEN,
      ...(env.R2_SQL_BASE_URL ? { baseUrl: env.R2_SQL_BASE_URL } : {}),
    });
    return route(request, env, sql);
  },
} satisfies ExportedHandler<Env>;
