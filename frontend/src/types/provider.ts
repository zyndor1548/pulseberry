export type Status = "Healthy" | "Degraded" | "Down";

export interface Provider {
  name: string;
  status: Status;
  score: number;
  latency: number | null;
  traffic: number;
}
