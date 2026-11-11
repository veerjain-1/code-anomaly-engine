import { useState, useCallback, useMemo } from 'react'
import { Shield, ShieldAlert, Clock, FileCode, Search, Download, X } from 'lucide-react'
import type { Anomaly } from '../hooks/useWebSocket'

interface AnomalyFeedProps {
  anomalies: Anomaly[]
  onSelect: (anomaly: Anomaly) => void
  selectedId?: string
}

type FilterType = 'all' | 'vulnerable' | 'clean'
type SortType = 'newest' | 'highest-risk' | 'lowest-risk' | 'latency'

export function AnomalyFeed({ anomalies, onSelect, selectedId }: AnomalyFeedProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [filter, setFilter] = useState<FilterType>('all')
  const [sortBy, setSortBy] = useState<SortType>('newest')

  const formatTime = useCallback((ts: number) => {
    const d = new Date(ts)
    return d.toLocaleTimeString('en-US', { hour12: false })
  }, [])

  const stats = useMemo(() => {
    const total = anomalies.length
    const vulnerable = anomalies.filter(a => a.is_anomalous).length
    return { total, vulnerable, clean: total - vulnerable }
  }, [anomalies])

  // Filter & Search Logic
  const filteredAnomalies = useMemo(() => {
    return anomalies
      .filter(anomaly => {
        // Status filter
        if (filter === 'vulnerable' && !anomaly.is_anomalous) return false
        if (filter === 'clean' && anomaly.is_anomalous) return false

        // Search query filter
        if (searchQuery.trim()) {
          const query = searchQuery.toLowerCase().trim()
          const matchesFile = anomaly.file_path.toLowerCase().includes(query)
          const matchesRepo = anomaly.repo.toLowerCase().includes(query)
          const matchesCommit = anomaly.commit_sha?.toLowerCase().includes(query)
          const matchesLabel = anomaly.label?.toLowerCase().includes(query)
          if (!matchesFile && !matchesRepo && !matchesCommit && !matchesLabel) {
            return false
          }
        }
        return true
      })
      .sort((a, b) => {
        if (sortBy === 'newest') return b.timestamp - a.timestamp
        if (sortBy === 'highest-risk') return b.confidence - a.confidence
        if (sortBy === 'lowest-risk') return a.confidence - b.confidence
        if (sortBy === 'latency') return b.latency_ms - a.latency_ms
        return 0
      })
  }, [anomalies, filter, searchQuery, sortBy])

  // Export filtered anomalies as JSON incident report
  const handleExport = useCallback(() => {
    const report = {
      generated_at: new Date().toISOString(),
      total_count: filteredAnomalies.length,
      vulnerable_count: filteredAnomalies.filter(a => a.is_anomalous).length,
      filter_applied: filter,
      search_query: searchQuery || 'none',
      anomalies: filteredAnomalies,
    }
    const blob = new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `code-anomaly-report-${Date.now()}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }, [filteredAnomalies, filter, searchQuery])

  return (
    <div className="anomaly-feed">
      <div className="feed-header">
        <h2>
          <ShieldAlert size={20} />
          Anomaly Feed
        </h2>
        <div className="feed-header-actions">
          <div className="feed-stats">
            <span className="stat stat-total">{stats.total} total</span>
            <span className="stat stat-vuln">{stats.vulnerable} vuln</span>
            <span className="stat stat-clean">{stats.clean} clean</span>
          </div>
          <button 
            className="feed-export-btn" 
            onClick={handleExport}
            title="Export filtered anomalies as JSON report"
            disabled={filteredAnomalies.length === 0}
          >
            <Download size={14} />
            <span>Export</span>
          </button>
        </div>
      </div>

      {/* Search & Filter Toolbar */}
      <div className="feed-toolbar">
        <div className="feed-search-box">
          <Search size={14} className="search-icon" />
          <input
            type="text"
            placeholder="Search file, repo, commit..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="feed-search-input"
          />
          {searchQuery && (
            <button className="search-clear-btn" onClick={() => setSearchQuery('')}>
              <X size={12} />
            </button>
          )}
        </div>

        <div className="feed-controls">
          <div className="feed-filter-pills">
            <button
              className={`filter-pill ${filter === 'all' ? 'active' : ''}`}
              onClick={() => setFilter('all')}
            >
              All ({stats.total})
            </button>
            <button
              className={`filter-pill vuln ${filter === 'vulnerable' ? 'active' : ''}`}
              onClick={() => setFilter('vulnerable')}
            >
              Vuln ({stats.vulnerable})
            </button>
            <button
              className={`filter-pill clean ${filter === 'clean' ? 'active' : ''}`}
              onClick={() => setFilter('clean')}
            >
              Clean ({stats.clean})
            </button>
          </div>

          <select
            value={sortBy}
            onChange={e => setSortBy(e.target.value as SortType)}
            className="feed-sort-select"
          >
            <option value="newest">Newest</option>
            <option value="highest-risk">Highest Risk</option>
            <option value="lowest-risk">Lowest Risk</option>
            <option value="latency">Highest Latency</option>
          </select>
        </div>
      </div>

      {/* Feed List */}
      <div className="feed-list">
        {anomalies.length === 0 ? (
          <div className="feed-empty">
            <Shield size={48} strokeWidth={1} />
            <p>No anomalies detected yet</p>
            <p className="feed-empty-sub">Push code to trigger analysis</p>
          </div>
        ) : filteredAnomalies.length === 0 ? (
          <div className="feed-empty">
            <Search size={40} strokeWidth={1} />
            <p>No matches found</p>
            <p className="feed-empty-sub">Try adjusting your search or filters</p>
            <button 
              className="feed-clear-filters-btn"
              onClick={() => { setSearchQuery(''); setFilter('all'); }}
            >
              Clear filters
            </button>
          </div>
        ) : (
          filteredAnomalies.map(anomaly => (
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
