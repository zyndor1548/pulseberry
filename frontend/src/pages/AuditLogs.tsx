import { useQuery } from "@tanstack/react-query";
import type { AuditLogEntry } from "../types/auditLog";
import { mapAuditLogFromApi } from "../types/auditLog";
import { metricsGateway } from "../services/apiGateway";
import { logsUrl } from "../services/urls";
import styles from "./AuditLogs.module.css";

const AuditLogs: React.FC = () => {
  const { data: logs, isLoading, isError, error } = useQuery({
    queryKey: ["audit-logs"],
    queryFn: async (): Promise<AuditLogEntry[]> => {
      const { data } = await metricsGateway.get<Parameters<typeof mapAuditLogFromApi>[0][]>(logsUrl);
      return Array.isArray(data) ? data.map(mapAuditLogFromApi) : [];
    },
  });

  return (
    <div className={styles.wrapper}>
      <header className={styles.header}>
        <h1 className={styles.title}>Audit logs</h1>
      </header>

      {isLoading && <p className={styles.message}>Loading audit logs…</p>}
      {isError && (
        <p className={styles.messageError}>
          Failed to load audit logs: {error instanceof Error ? error.message : "Unknown error"}
        </p>
      )}
      {!isLoading && !isError && logs && (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>ID</th>
                <th>Gateway</th>
                <th>Status</th>
                <th>Latency</th>
                <th>Timestamp</th>
              </tr>
            </thead>
            <tbody>
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={5} className={styles.empty}>
                    No audit logs yet.
                  </td>
                </tr>
              ) : (
                logs.map((row) => (
                  <tr key={`${row.id}-${row.gateway}-${row.timestamp}`}>
                    <td className={styles.id}>{row.id}</td>
                    <td>{row.gateway}</td>
                    <td>
                      <span className={`${styles.status} ${styles[row.status.toLowerCase()]}`}>
                        {row.status}
                      </span>
                    </td>
                    <td>{row.latency !== null ? `${row.latency} ms` : "—"}</td>
                    <td className={styles.timestamp}>{row.timestamp}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};

export default AuditLogs;
