import { useMemo } from 'react'
import { ShieldAlert, Shield, GitCommit, FileCode, MapPin } from 'lucide-react'
import type { Anomaly } from '../hooks/useWebSocket'

interface DiffViewerProps {
  anomaly: Anomaly | null
}

export function DiffViewer({ anomaly }: DiffViewerProps) {
  const lines = useMemo(() => {
    if (!anomaly?.code) return []
    return anomaly.code.split('\n').map((line, i) => ({
      number: anomaly.start_line + i,
      content: line,
    }))
  }, [anomaly])

  if (!anomaly) {
    return (
      <div className="diff-viewer diff-viewer-empty">
        <FileCode size={64} strokeWidth={1} />
        <p>Select an anomaly to view code</p>
      </div>
    )
  }

  return (
    <div className="diff-viewer">
      <div className="diff-header">
        <div className="diff-title">
          {anomaly.is_anomalous ? (
            <ShieldAlert size={20} className="icon-vuln" />
          ) : (
            <Shield size={20} className="icon-clean" />
          )}
          <h3>{anomaly.file_path}</h3>
          <span className={`diff-badge ${anomaly.is_anomalous ? 'badge-vuln' : 'badge-clean'}`}>
            {anomaly.label} — {(anomaly.confidence * 100).toFixed(1)}%
          </span>
        </div>
        <div className="diff-meta">
          <span><GitCommit size={14} /> {anomaly.commit_sha.slice(0, 8)}</span>
          <span>{anomaly.repo}</span>
          <span><MapPin size={14} /> L{anomaly.start_line}–L{anomaly.end_line}</span>
        </div>
      </div>

      <div className="diff-code">
        <pre>
          <code>
            {lines.map((line, i) => (
              <div
                key={i}
                className={`code-line ${anomaly.is_anomalous ? 'code-line-vuln' : ''}`}
              >
                <span className="line-number">{line.number}</span>
                <span className="line-content">{line.content || ' '}</span>
              </div>
            ))}
          </code>
        </pre>
      </div>
    </div>
  )
}
