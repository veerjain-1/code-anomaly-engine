import { useCallback, useMemo } from 'react'
import { Shield, ShieldAlert, Clock, FileCode } from 'lucide-react'
import type { Anomaly } from '../hooks/useWebSocket'

interface AnomalyFeedProps {
  anomalies: Anomaly[]
  onSelect: (anomaly: Anomaly) => void
  selectedId?: string
}

export function AnomalyFeed({ anomalies, onSelect, selectedId }: AnomalyFeedProps) {
  const formatTime = useCallback((ts: number) => {
    const d = new Date(ts)
    return d.toLocaleTimeString('en-US', { hour12: false })
  }, [])

  const stats = useMemo(() => {
    const total = anomalies.length
    const vulnerable = anomalies.filter(a => a.is_anomalous).length
    return { total, vulnerable, clean: total - vulnerable }
  }, [anomalies])

  return (
    <div className="anomaly-feed">
      <div className="feed-header">
        <h2>
          <ShieldAlert size={20} />
          Anomaly Feed
        </h2>
        <div className="feed-stats">
          <span className="stat stat-total">{stats.total} analyzed</span>
          <span className="stat stat-vuln">{stats.vulnerable} vulnerable</span>
          <span className="stat stat-clean">{stats.clean} clean</span>
        </div>
      </div>

      <div className="feed-list">
        {anomalies.length === 0 ? (
          <div className="feed-empty">
            <Shield size={48} strokeWidth={1} />
            <p>No anomalies detected yet</p>
            <p className="feed-empty-sub">Push code to trigger analysis</p>
          </div>
        ) : (
          anomalies.map(anomaly => (
            <button
              key={anomaly.id}
              className={`feed-item ${anomaly.is_anomalous ? 'feed-item-vuln' : 'feed-item-clean'} ${selectedId === anomaly.id ? 'feed-item-selected' : ''}`}
              onClick={() => onSelect(anomaly)}
            >
              <div className="feed-item-icon">
                {anomaly.is_anomalous ? (
                  <ShieldAlert size={18} className="icon-vuln" />
                ) : (
                  <Shield size={18} className="icon-clean" />
                )}
              </div>
              <div className="feed-item-content">
                <div className="feed-item-header">
                  <span className="feed-item-file">
                    <FileCode size={14} />
                    {anomaly.file_path}
                  </span>
                  <span className={`feed-item-badge ${anomaly.is_anomalous ? 'badge-vuln' : 'badge-clean'}`}>
                    {anomaly.label}
                  </span>
                </div>
                <div className="feed-item-meta">
                  <span className="feed-item-repo">{anomaly.repo}</span>
                  <span className="feed-item-confidence">
                    {(anomaly.confidence * 100).toFixed(1)}% confidence
                  </span>
                  <span className="feed-item-latency">
                    <Clock size={12} />
                    {anomaly.latency_ms.toFixed(1)}ms
                  </span>
                  <span className="feed-item-time">
                    {formatTime(anomaly.timestamp)}
                  </span>
                </div>
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  )
}
