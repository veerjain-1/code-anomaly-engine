import { useState, useEffect } from 'react'
import { AnomalyFeed } from './components/AnomalyFeed'
import { DiffViewer } from './components/DiffViewer'
import { MetricsPanel } from './components/MetricsPanel'
import { useWebSocket, type Anomaly } from './hooks/useWebSocket'
import { Terminal } from 'lucide-react'

// Allow overriding via env var, default to localhost for dev
const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'
const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws/feed'

function App() {
  const [anomalies, setAnomalies] = useState<Anomaly[]>([])
  const [selectedAnomaly, setSelectedAnomaly] = useState<Anomaly | null>(null)

  // Fetch initial anomalies on load
  useEffect(() => {
    fetch(`${API_URL}/api/anomalies`)
      .then(res => res.json())
      .then(data => {
        if (data.anomalies) {
          setAnomalies(data.anomalies)
          if (data.anomalies.length > 0) {
            setSelectedAnomaly(data.anomalies[0])
          }
        }
      })
      .catch(err => console.error('Failed to fetch initial anomalies:', err))
  }, [])

  // Listen for real-time updates via WebSocket
  const { isConnected } = useWebSocket({
    url: WS_URL,
    onMessage: (newAnomaly) => {
      setAnomalies(prev => [newAnomaly, ...prev].slice(0, 500)) // Keep last 500
      
      // Auto-select if it's the first one or if it's a vulnerability and we're just looking at a clean one
      setSelectedAnomaly(current => {
        if (!current) return newAnomaly
        if (newAnomaly.is_anomalous && !current.is_anomalous) return newAnomaly
        return current
      })
    }
  })

  return (
    <div className="app">
      <header className="top-nav">
        <div className="brand">
          <Terminal size={24} className="brand-icon" />
          <h1>Code Anomaly Engine</h1>
        </div>
        <div className="connection-status">
          <div className={`status-dot ${isConnected ? 'status-connected' : 'status-disconnected'}`} />
          {isConnected ? 'Live Feed Active' : 'Connecting...'}
        </div>
      </header>

      <main className="dashboard-grid">
        <aside className="sidebar">
          <AnomalyFeed 
            anomalies={anomalies} 
            onSelect={setSelectedAnomaly} 
            selectedId={selectedAnomaly?.id}
          />
        </aside>
        
        <section className="main-content">
          <div className="viewer-container">
            <DiffViewer anomaly={selectedAnomaly} />
          </div>
          <div className="metrics-container">
            <MetricsPanel anomalies={anomalies} />
          </div>
        </section>
      </main>
    </div>
  )
}

export default App
