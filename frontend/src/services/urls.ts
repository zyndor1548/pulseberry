export const metricsUrls = {
  metrics: () => "/api/metrics",
};

export const logsUrl = import.meta.env.DEV ? "/api/logs" : "/logs";

const admin = import.meta.env.DEV ? "/api/admin" : "/admin";
export const adminUrls = {
  enable: (provider: string) => `${admin}/providers/enable?provider=${encodeURIComponent(provider)}`,
  disable: (provider: string) => `${admin}/providers/disable?provider=${encodeURIComponent(provider)}`,
};  