import { motion } from "framer-motion";
import type { Status } from "../../types/provider";
import styles from "./DashboardCard.module.css";

interface DashboardCardProps {
  name: string;
  status: Status;
  score: number;
  latency: number | null;
  traffic: number;
  index: number;
}

const DashboardCard: React.FC<DashboardCardProps> = ({
  name,
  status,
  score,
  latency,
  traffic,
  index,
}) => {
  return (
    <motion.article
      className={`${styles.card} ${styles[status.toLowerCase()]}`}
      initial={{ y: 20, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ duration: 0.4, delay: index * 0.1 }}
      aria-label={`${name}, ${status}`}
    >
      <div className={styles.header}>
        <h3>{name}</h3>
        <span
          className={styles.statusBadge}
          role="status"
          aria-label={`Status: ${status}`}
        >
          <span className={styles.statusDot} aria-hidden />
          {status}
        </span>
      </div>

      <div className={styles.metrics}>
        <div className={styles.metric}>
          <span className={styles.label}>Score</span>
          <span className={styles.value}>{score}%</span>
        </div>

        <div className={styles.metric}>
          <span className={styles.label}>Latency</span>
          <span className={styles.value}>
            {latency !== null ? `${latency} ms` : "—"}
          </span>
        </div>

        <div className={styles.metric}>
          <span className={styles.label}>Traffic</span>
          <span className={styles.value}>{traffic}/min</span>
        </div>
      </div>
    </motion.article>
  );
};

export default DashboardCard;