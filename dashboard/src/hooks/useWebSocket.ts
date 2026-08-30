import { useState, useEffect, useRef, useCallback } from 'react'

export interface Anomaly {
  id: string
  repo: string
  commit_sha: string
  file_path: string
  start_line: number
  end_line: number
  code: string
  is_anomalous: boolean
  confidence: number
  label: string
  latency_ms: number
  timestamp: number
}

interface UseWebSocketOptions {
  url: string
  onMessage?: (anomaly: Anomaly) => void
  reconnectInterval?: number
}

export function useWebSocket({ url, onMessage, reconnectInterval = 3000 }: UseWebSocketOptions) {
  const [isConnected, setIsConnected] = useState(false)
  const [lastMessage, setLastMessage] = useState<Anomaly | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()

  const connect = useCallback(() => {
    try {
      const ws = new WebSocket(url)

      ws.onopen = () => {
        setIsConnected(true)
        console.log('[WS] Connected to', url)
      }

      ws.onmessage = (event) => {
        try {
          const anomaly: Anomaly = JSON.parse(event.data)
          setLastMessage(anomaly)
          onMessage?.(anomaly)
        } catch (err) {
          console.error('[WS] Failed to parse message:', err)
        }
      }

      ws.onclose = () => {
        setIsConnected(false)
        console.log('[WS] Disconnected, reconnecting in', reconnectInterval, 'ms')
        reconnectTimer.current = setTimeout(connect, reconnectInterval)
      }

      ws.onerror = (err) => {
        console.error('[WS] Error:', err)
        ws.close()
      }

      wsRef.current = ws
    } catch (err) {
      console.error('[WS] Connection failed:', err)
      reconnectTimer.current = setTimeout(connect, reconnectInterval)
    }
  }, [url, onMessage, reconnectInterval])

  useEffect(() => {
    connect()
    return () => {
      clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connect])

  return { isConnected, lastMessage }
}
