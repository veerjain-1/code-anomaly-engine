import { useMemo } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { Activity } from 'lucide-react'
import type { Anomaly } from '../hooks/useWebSocket'

interface MetricsPanelProps {
  anomalies: Anomaly[]
}

export function MetricsPanel({ anomalies }: MetricsPanelProps) {
  const chartData = useMemo(() => {
    // Group anomalies by minute to show latency over time
    const groups = new Map<number, number[]>()
    
    anomalies.forEach(a => {
      // Bucket into 10-second intervals for smoother charts
      const bucket = Math.floor(a.timestamp / 10000) * 10000
      if (!groups.has(bucket)) groups.set(bucket, [])
      groups.get(bucket)?.push(a.latency_ms)
    })

    return Array.from(groups.entries())
      .map(([ts, latencies]) => ({
        time: new Date(ts).toLocaleTimeString('en-US', { hour12: false, minute: '2-digit', second: '2-digit' }),
        latency: latencies.reduce((a, b) => a + b, 0) / latencies.length, // average
        p99: [...latencies].sort((a, b) => a - b)[Math.floor(latencies.length * 0.99)] || latencies[0],
      }))
      .sort((a, b) => a.time.localeCompare(b.time))
      .slice(-30) // Show last 30 data points
  }, [anomalies])

  return (
    <div className="metrics-panel">
      <div className="metrics-header">
        <h2>
          <Activity size={20} />
          Inference Latency
        </h2>
      </div>
      
      <div className="chart-container">
        {chartData.length === 0 ? (
          <div className="chart-empty">Waiting for inference data...</div>
        ) : (
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#333" />
              <XAxis dataKey="time" stroke="#888" tick={{ fill: '#888', fontSize: 12 }} />
              <YAxis stroke="#888" tick={{ fill: '#888', fontSize: 12 }} unit="ms" />
              <Tooltip 
                contentStyle={{ backgroundColor: '#1e1e1e', border: '1px solid #333' }}
                itemStyle={{ color: '#fff' }}
              />
              <Line 
                type="monotone" 
                name="Avg Latency"
                dataKey="latency" 
                stroke="#61DAFB" 
                strokeWidth={2}
                dot={false}
              />
              <Line 
                type="monotone" 
                name="P99 Latency"
                dataKey="p99" 
                stroke="#DEA584" 
                strokeWidth={2}
                dot={false}
                strokeDasharray="5 5"
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}
