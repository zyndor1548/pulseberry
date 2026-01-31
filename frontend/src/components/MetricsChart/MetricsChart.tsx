import React from "react";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";

export interface HistoryPoint {
  timestamp: string;
  providers: { name: string; score: number; latency: number | null }[];
}

export interface ProviderSnapshot {
  name: string;
  score: number;
  latency: number | null;
}

const COLORS = [
  "#f59e0b", 
  "#a855f7", 
  "#ef4444", 
];

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString("en-IN", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
    });
  } catch {
    return iso;
  }
}

interface MetricsChartProps {
  history: HistoryPoint[];
  currentProviders?: ProviderSnapshot[];
}

export const MetricsChart: React.FC<MetricsChartProps> = ({
  history,
  currentProviders = [],
}) => {
  const names =
    history.length > 0
      ? [...new Set(history.flatMap((h) => h.providers.map((p) => p.name)))].sort()
      : currentProviders.map((p) => p.name);

  const scoreData = history.map(({ timestamp, providers }) => {
    const row: Record<string, string | number> = { time: formatTime(timestamp) };
    providers.forEach((p) => {
      row[p.name] = p.score;
    });
    return row;
  });

  const hasTrends = history.length > 0 && names.length > 0;
  const hasCurrent = currentProviders.length > 0;
  const barData = currentProviders.map((p, i) => ({
    gateway: p.name,
    score: p.score,
    fill: COLORS[i % COLORS.length],
  }));

  if (!hasTrends && hasCurrent) {
    return (
      <div style={styles.singleChart}>
        <h3 style={styles.chartTitle}>Gateway score</h3>
        <ResponsiveContainer width="100%" height={320}>
          <BarChart data={barData} margin={{ top: 12, right: 24, left: 0, bottom: 8 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" vertical={false} />
            <XAxis dataKey="gateway" tick={{ fontSize: 12 }} stroke="#6b7280" />
            <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} stroke="#6b7280" />
            <Tooltip
              contentStyle={{ borderRadius: 8, border: "1px solid #e5e7eb" }}
              formatter={(value: number) => [`${value}%`, "Score"]}
            />
            <Legend />
            <Bar dataKey="score" name="Score" radius={[6, 6, 0, 0]}>
              {barData.map((entry) => (
                <Cell key={entry.gateway} fill={entry.fill} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    );
  }

  return (
    <div style={styles.singleChart}>
      <h3 style={styles.chartTitle}>Gateway score over time</h3>
      <ResponsiveContainer width="100%" height={320}>
        <LineChart data={scoreData} margin={{ top: 5, right: 24, left: 0, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="time" tick={{ fontSize: 11 }} stroke="#6b7280" />
          <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} stroke="#6b7280" />
          <Tooltip
            contentStyle={{ borderRadius: 8, border: "1px solid #e5e7eb" }}
            labelStyle={{ fontWeight: 600 }}
            formatter={(value: number) => [`${value}%`, "Score"]}
          />
          <Legend />
          {names.map((name, i) => (
            <Line
              key={name}
              type="monotone"
              dataKey={name}
              stroke={COLORS[i % COLORS.length]}
              strokeWidth={3}
              dot={false}
              name={name}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
};

const styles: Record<string, React.CSSProperties> = {
  singleChart: {
    minHeight: 320,
  },
  chartTitle: {
    marginBottom: "0.25rem",
    fontSize: "1.05rem",
    fontWeight: 600,
    color: "#1f2937",
  },
  hint: {
    marginBottom: "0.75rem",
    fontSize: "0.85rem",
    color: "#6b7280",
    marginTop: 0,
  },
  empty: {
    padding: "3rem 2rem",
    textAlign: "center",
    color: "#6b7280",
    fontSize: "0.95rem",
  },
};

export default MetricsChart;
