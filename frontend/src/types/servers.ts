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

export interface CircuitBreaker {
  error_count: number;
  error_rate: string;
  failure_count: number;
  name: string;
  state: string;
  success_count: number;
  total_requests: number;
}

export interface ComplianceProvider {
  enabled: boolean;
  name: string;
}

export interface PaymentProvider {
  capabilities: Record<string, unknown>;
  circuit_breaker: CircuitBreaker;
  enabled: boolean;
  name: string;
  priority: number;
}

export interface ProviderRegistry {
  compliance_providers: ComplianceProvider[];
  payment_providers: PaymentProvider[];
}

export interface ServersResponse {
  provider_registry?: ProviderRegistry;
  server_count: number;
  servers: Server[];
  timestamp: string;
}
