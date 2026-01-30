import DashboardCard from "../components/DashboardCard/DashboardCard";
import type { Provider } from "../types/provider";
import styles from "./Home.module.css";

const providers: Provider[] = [
  {
    name: "Stripe",
    status: "Healthy",
    score: 98,
    latency: 120,
    traffic: 3400,
  },
  {
    name: "Razorpay",
    status: "Degraded",
    score: 72,
    latency: 480,
    traffic: 2100,
  },
  {
    name: "PayPal",
    status: "Down",
    score: 0,
    latency: null,
    traffic: 0,
  },
];

const Home: React.FC = () => {
  return (
    <div className={styles.wrapper}>
      <header className={styles.header}>
        <h1 className={styles.title}>Provider health</h1>
      </header>
      <div className={styles.grid}>
        {providers.map((p, index) => (
          <DashboardCard key={p.name} {...p} index={index} />
        ))}
      </div>
    </div>
  );
};

export default Home;