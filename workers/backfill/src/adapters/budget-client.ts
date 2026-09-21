import type { DiscordHttpBudgetDurableObject } from "../do/discord-http-budget";
import type { BudgetDecision, BudgetGate, RequestReport } from "../ports";

/** Forwards budget coordination to the application-wide Budget Durable Object over RPC. */
export class BudgetDurableObjectClient implements BudgetGate {
  constructor(private readonly stub: DurableObjectStub<DiscordHttpBudgetDurableObject>) {}

  acquire(): Promise<BudgetDecision> {
    return this.stub.acquire();
  }

  report(report: RequestReport): Promise<void> {
    return this.stub.report(report);
  }
}
