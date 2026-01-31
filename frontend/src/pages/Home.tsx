import { useQuery } from "@tanstack/react-query";
import DashboardCard from "../components/DashboardCard/DashboardCard";
import type { Provider } from "../types/provider";
import { fetchServers } from "../api/servers";
import { serverToProvider } from "../utils/serverToProvider";
import styles from "./Home.module.css";

const Home: React.FC = () => {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["servers", "metrics"],
    queryFn: fetchServers,
  });

  const providers: Provider[] = data?.servers?.map(serverToProvider) ?? [];

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
        <div className={styles.grid}>
          {providers.map((p, index) => (
            <DashboardCard key={p.name} {...p} index={index} />
          ))}
        </div>
      )}
    </div>
  );
};

export default Home;