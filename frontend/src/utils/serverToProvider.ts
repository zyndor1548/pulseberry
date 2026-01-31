import type { Provider, Status } from "../types/provider";
import type {
  Server,
  ComplianceProvider,
  PaymentProvider,
} from "../types/servers";

function nameFromUrl(serverUrl: string): string {
  try {
    const path = new URL(serverUrl).pathname.replace(/\/$/, "");
    const segment = path.split("/").filter(Boolean).pop();
    return segment || serverUrl;
  } catch {
    return serverUrl;
  }
}

function statusFromServer(s: Server): Status {
  if (s.score <= 0 || s.gateway_errors > 0 || s.network_errors > 0) return "Down";
  if (s.score >= 90) return "Healthy";
  return "Degraded";
}

export function serverToProvider(server: Server): Provider {
  return {
    name: server.name || nameFromUrl(server.server_url),
    status: statusFromServer(server),
    score: server.score,
    latency: server.total_requests > 0 ? server.avg_latency_ms : null,
    traffic: server.total_requests,
  };
}

function statusFromCircuitBreaker(enabled: boolean, state: string): Status {
  if (!enabled) return "Down";
  if (state === "OPEN") return "Down";
  if (state === "HALF_OPEN") return "Degraded";
  return "Healthy";
}

function scoreFromErrorRate(errorRate: string): number {
  const pct = parseFloat(errorRate.replace("%", "")) || 0;
  return Math.round(100 - pct);
}

function complianceToProvider(c: ComplianceProvider): Provider {
  return {
    name: c.name,
    status: c.enabled ? "Healthy" : "Down",
    score: 100,
    latency: null,
    traffic: 0,
  };
}

function paymentToProvider(p: PaymentProvider): Provider {
  const cb = p.circuit_breaker;
  return {
    name: p.name,
    status: statusFromCircuitBreaker(p.enabled, cb.state),
    score: scoreFromErrorRate(cb.error_rate),
    latency: null,
    traffic: cb.total_requests,
  };
}

/** Map provider_registry to Provider[] (onfido, stripe, klarna, razorpay, etc.) */
export function registryToProviders(
  registry: { compliance_providers: ComplianceProvider[]; payment_providers: PaymentProvider[] }
): Provider[] {
  const fromCompliance = registry.compliance_providers.map(complianceToProvider);
  const fromPayment = registry.payment_providers.map(paymentToProvider);
  return [...fromCompliance, ...fromPayment];
}
