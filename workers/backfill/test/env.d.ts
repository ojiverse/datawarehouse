import type { Env as WorkerEnv } from "../src/env";

// The Cloudflare Vitest plugin types `env` as the global `Cloudflare.Env`; this augmentation
// binds it to the bindings declared in wrangler.jsonc without a generated worker-configuration.d.ts.
declare global {
  namespace Cloudflare {
    interface Env extends WorkerEnv {}
  }
}
