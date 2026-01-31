export interface AuditLogEntry {
  id: string;
  gateway: string;
  status: "Success" | "Failed" | "Retracked";
  latency: number | null;
  timestamp: string;
}

/** Raw row from the /logs API */
export interface AuditLogApiRow {
  transaction_id: number;
  link: string;
  name: string;
  status: 0 | 1;
  latency: number;
  current_time: number;
}

export function mapAuditLogFromApi(row: AuditLogApiRow): AuditLogEntry {
  const status: AuditLogEntry["status"] = row.status === 0 ? "Success" : "Failed";
  const date = new Date(row.current_time * 1000);
  const timestamp = date.toISOString().replace("T", " ").slice(0, 19);
  return {
    id: String(row.transaction_id),
    gateway: row.name,
    status,
    latency: row.latency,
    timestamp,
  };
}
