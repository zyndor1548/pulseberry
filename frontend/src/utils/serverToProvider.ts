import type { Provider, Status } from "../types/provider";
import type { Server } from "../types/servers";

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
    name: nameFromUrl(server.server_url),
    status: statusFromServer(server),
    score: server.score,
    latency: server.total_requests > 0 ? server.avg_latency_ms : null,
    traffic: server.total_requests,
  };
}
