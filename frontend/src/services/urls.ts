export const metricsUrls = {
  metrics: () => "/api/metrics",
};

export const logsUrl = import.meta.env.DEV ? "/api/logs" : "/logs";  