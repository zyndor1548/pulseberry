export interface Server {
  active_connections: number;
  avg_latency_ms: number;
  bank_errors: number;
  gateway_errors: number;
  last_updated: string;
  name: string;
  network_errors: number;
  score: number;
  server_url: string;
  success_rate: number;
  total_requests: number;
}

export interface ServersResponse {
  server_count: number;
  servers: Server[];
  timestamp: string;
}
