import { adminUrls } from "../services/urls";

const postOptions: RequestInit = {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: "{}",
};

export async function enableProvider(provider: string): Promise<void> {
  const res = await fetch(adminUrls.enable(provider), postOptions);
  if (!res.ok) throw new Error(`Enable failed: ${res.status}`);
}

export async function disableProvider(provider: string): Promise<void> {
  const res = await fetch(adminUrls.disable(provider), postOptions);
  if (!res.ok) throw new Error(`Disable failed: ${res.status}`);
}

export async function resetCircuitBreaker(provider: string): Promise<void> {
  const res = await fetch(adminUrls.resetCircuit(provider), postOptions);
  if (!res.ok) throw new Error(`Reset failed: ${res.status}`);
}
