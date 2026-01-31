import type { ServersResponse } from "../types/servers";
import { metricsUrls } from "../services/urls";

/**
 * Fetch metrics via /api/metrics. In dev, Vite proxies this to ngrok with
 * ngrok-skip-browser-warning so we get JSON instead of the interstitial HTML.
 * Same-origin request = no CORS, no preflight.
 */
export const fetchServers = async (): Promise<ServersResponse> => {
  const url = metricsUrls.metrics();
  const res = await fetch(url, { method: "GET" });
  if (!res.ok) {
    throw new Error(`Metrics error: ${res.status} ${res.statusText}`);
  }
  const data = await res.json();
  if (data?.servers === undefined && !data?.provider_registry) {
    throw new Error("Invalid metrics response");
  }
  return data as ServersResponse;
};
