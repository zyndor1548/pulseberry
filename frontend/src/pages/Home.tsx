import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import DashboardCard from "../components/DashboardCard/DashboardCard";
import MetricsChart, { type HistoryPoint } from "../components/MetricsChart/MetricsChart";
import type { Provider } from "../types/provider";
import { fetchServers } from "../api/servers";
import { enableProvider, disableProvider, resetCircuitBreaker } from "../api/admin";
import { serverToProvider } from "../utils/serverToProvider";
import styles from "./Home.module.css";

const MAX_HISTORY = 40;

const Home: React.FC = () => {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["servers", "metrics"],
    queryFn: fetchServers,
    refetchInterval: 3000,
    refetchIntervalInBackground: true,
  });

  const providers: Provider[] = data?.servers?.map(serverToProvider) ?? [];
  const sortedProviders = [...providers].sort((a, b) => a.name.localeCompare(b.name));
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const lastTimestamp = useRef<string | null>(null);

  const [storeEnabled, setStoreEnabled] = useState<Record<string, boolean>>({});
  const enabledByStore = (name: string) => storeEnabled[name] !== false;

  const handleSetEnabled = async (name: string, enabled: boolean) => {
    try {
      if (enabled) await enableProvider(name);
      else await disableProvider(name);
      setStoreEnabled((prev) => ({ ...prev, [name]: enabled }));
    } catch (e) {
      alert(e instanceof Error ? e.message : "Request failed");
    }
  };

  const handleReset = async (name: string) => {
    try {
      await resetCircuitBreaker(name);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Reset failed");
    }
  };

  useEffect(() => {
    if (!data?.timestamp || !data?.servers?.length) return;
    if (lastTimestamp.current === data.timestamp) return;
    lastTimestamp.current = data.timestamp;
    const current = data.servers.map(serverToProvider);
    const point: HistoryPoint = {
      timestamp: data.timestamp,
      providers: current.map((p) => ({
        name: p.name,
        score: p.score,
        latency: p.latency,
      })),
    };
    setHistory((prev) => [...prev.slice(-(MAX_HISTORY - 1)), point]);
  }, [data]);

  return (
    <div className={styles.wrapper}>
      <header className={styles.header}>
        <h1 className={styles.title}>Provider health</h1>
      </header>

      {isLoading && <p className={styles.message}>Loading…</p>}
      {isError && (
        <p className={styles.error} role="alert">
          {error instanceof Error ? error.message : "Failed to load metrics"}
        </p>
      )}
      {!isLoading && !isError && (
        <>
          <div className={styles.grid}>
            {sortedProviders.map((p, index) => (
              <DashboardCard
                key={p.name}
                {...p}
                index={index}
                enabled={enabledByStore(p.name)}
                onSetEnabled={(enabled) => handleSetEnabled(p.name, enabled)}
                onReset={() => handleReset(p.name)}
              />
            ))}
          </div>
          <section className={styles.chartSection} aria-label="Metrics over time">
            <h2 className={styles.sectionTitle}>Live metrics: Gateway score over time</h2>
            <MetricsChart
              history={history}
              currentProviders={sortedProviders.map((p) => ({
                name: p.name,
                score: p.score,
                latency: p.latency,
              }))}
            />
          </section>
        </>
      )}
    </div>
  );
};

export default Home;