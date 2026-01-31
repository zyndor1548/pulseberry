export function getScoreFromMetrics(
  latencyMs: number | null,
  trafficPerMin: number
): number {
  if (latencyMs === null || latencyMs <= 0) return 0;

  const latencyScore = Math.max(0, 100 - (latencyMs / 10)); 
  const trafficScore = Math.min(100, (trafficPerMin / 50));

  return Math.round(0.6 * latencyScore + 0.4 * trafficScore);
}
