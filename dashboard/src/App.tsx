import { useState, useEffect, useRef } from 'react'
import { AnomalyFeed } from './components/AnomalyFeed'
import { DiffViewer } from './components/DiffViewer'
import { MetricsPanel } from './components/MetricsPanel'
import { useWebSocket, type Anomaly } from './hooks/useWebSocket'
import { Terminal, Sparkles, Play, Pause } from 'lucide-react'
import { INITIAL_MOCK_ANOMALIES, generateMockAnomaly } from './data/mockData'

// Allow overriding via env var, default to localhost for dev
const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'
const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws/feed'

function App() {
  const [anomalies, setAnomalies] = useState<Anomaly[]>(INITIAL_MOCK_ANOMALIES)
  const [selectedAnomaly, setSelectedAnomaly] = useState<Anomaly | null>(INITIAL_MOCK_ANOMALIES[0] || null)
  const [isSimulating, setIsSimulating] = useState<boolean>(false)
  const simIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Fetch initial anomalies on load from backend if available
  useEffect(() => {
    fetch(`${API_URL}/api/anomalies`)
      .then(res => res.json())
      .then(data => {
        if (data.anomalies && data.anomalies.length > 0) {
          setAnomalies(data.anomalies)
          setSelectedAnomaly(data.anomalies[0])
        }
      })
      .catch(_err => {
        // Backend not running locally; keep INITIAL_MOCK_ANOMALIES
      })
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

  // Handle Simulation Mode streaming
  useEffect(() => {
    if (isSimulating) {
      simIntervalRef.current = setInterval(() => {
        const mockItem = generateMockAnomaly()
        setAnomalies(prev => [mockItem, ...prev].slice(0, 500))
        setSelectedAnomaly(current => {
          if (!current || (mockItem.is_anomalous && !current.is_anomalous)) {
            return mockItem
          }
          return current
        })
      }, 6000)
    } else if (simIntervalRef.current) {
      clearInterval(simIntervalRef.current)
      simIntervalRef.current = null
    }

    return () => {
      if (simIntervalRef.current) {
        clearInterval(simIntervalRef.current)
      }
    }
  }, [isSimulating])

  return (
    <div className="app">
      <header className="top-nav">
        <div className="brand">
          <Terminal size={24} className="brand-icon" />
          <h1>Code Anomaly Engine</h1>
        </div>

        <div className="nav-actions">
          <button
            className={`sim-toggle-btn ${isSimulating ? 'sim-active' : ''}`}
            onClick={() => setIsSimulating(prev => !prev)}
            title="Toggle live anomaly stream simulation"
          >
            {isSimulating ? <Pause size={14} /> : <Play size={14} />}
            <Sparkles size={14} />
            <span>{isSimulating ? 'Streaming Demo (Live)' : 'Start Demo Stream'}</span>
          </button>

          <div className="connection-status">
            <div className={`status-dot ${isConnected ? 'status-connected' : isSimulating ? 'status-simulated' : 'status-disconnected'}`} />
            {isConnected ? 'Live WebSocket Active' : isSimulating ? 'Simulation Active' : 'Offline (Demo Mode)'}
          </div>
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

