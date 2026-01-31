import { useQuery } from "@tanstack/react-query";
import type { AuditLogEntry } from "../types/auditLog";
import { mapAuditLogFromApi } from "../types/auditLog";
import { metricsGateway } from "../services/apiGateway";
import { logsUrl } from "../services/urls";
import styles from "./AuditLogs.module.css";

function escapeCsv(val: string | number): string {
  const s = String(val);
  if (s.includes(",") || s.includes('"') || s.includes("\n")) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

function logsToCsv(logs: AuditLogEntry[]): string {
  const header = "ID,Gateway,Status,Latency (ms),Timestamp";
  const rows = logs.map((r) =>
    [r.id, r.gateway, r.status, r.latency ?? "", r.timestamp].map(escapeCsv).join(",")
  );
  return [header, ...rows].join("\n");
}

function downloadCsv(logs: AuditLogEntry[]) {
  const csv = logsToCsv(logs);
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

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
        {!isLoading && !isError && logs && logs.length > 0 && (
          <button
            type="button"
            className={styles.exportBtn}
            onClick={() => downloadCsv(logs)}
          >
            Export CSV
          </button>
        )}
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
