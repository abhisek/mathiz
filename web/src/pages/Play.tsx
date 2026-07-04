import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { api, deviceToken } from '../api'

type Status = 'connecting' | 'live' | 'ended' | 'error'

export default function Play() {
  const navigate = useNavigate()
  const hostRef = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<Status>('connecting')
  const [message, setMessage] = useState('')
  const [childName, setChildName] = useState('')
  const [reconnectNonce, setReconnectNonce] = useState(0)

  useEffect(() => {
    const token = deviceToken.get()
    if (!token) {
      navigate('/join')
      return
    }
    void api
      .childMe(token)
      .then((me) => setChildName(me.profile.name))
      .catch(() => {
        // Token was revoked — back to the join flow.
        deviceToken.clear()
        navigate('/join')
      })
  }, [navigate, reconnectNonce])

  useEffect(() => {
    const token = deviceToken.get()
    if (!token || !hostRef.current) return

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 16,
      fontFamily: '"JetBrains Mono", "Fira Code", Menlo, Consolas, monospace',
      theme: {
        background: '#16121f',
        foreground: '#e6e1f0',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(hostRef.current)
    fit.fit()
    term.focus()

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${window.location.host}/api/v1/terminal`)
    ws.binaryType = 'arraybuffer'
    setStatus('connecting')

    ws.onopen = () => {
      ws.send(
        JSON.stringify({ type: 'auth', token, cols: term.cols, rows: term.rows }),
      )
    }

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        try {
          const msg = JSON.parse(ev.data) as { type: string; message?: string }
          if (msg.type === 'ready') setStatus('live')
          if (msg.type === 'exit') setStatus('ended')
          if (msg.type === 'error') {
            setStatus('error')
            setMessage(msg.message ?? 'Connection error')
          }
        } catch {
          // ignore malformed control frames
        }
        return
      }
      term.write(new Uint8Array(ev.data as ArrayBuffer))
    }

    ws.onclose = () => {
      setStatus((s) => (s === 'error' ? s : 'ended'))
    }

    const dataSub = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

    const onResize = () => {
      fit.fit()
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      dataSub.dispose()
      ws.close()
      term.dispose()
    }
  }, [navigate, reconnectNonce])

  return (
    <div className="play-page">
      <header className="play-bar">
        <div className="brand brand-small brand-dark">
          <span className="brand-mark">∑</span>
          <span>Mathiz</span>
          {childName && <span className="muted">· {childName}</span>}
        </div>
        <div className="play-bar-right">
          {status === 'connecting' && <span className="pill pill-wait">connecting…</span>}
          {status === 'live' && <span className="pill pill-live">live</span>}
          {(status === 'ended' || status === 'error') && (
            <button
              className="btn btn-kid"
              onClick={() => setReconnectNonce((n) => n + 1)}
            >
              Play again
            </button>
          )}
          <button
            className="btn btn-ghost btn-ghost-dark"
            onClick={() => {
              deviceToken.clear()
              navigate('/join')
            }}
          >
            Switch player
          </button>
        </div>
      </header>

      <div className="term-wrap">
        <div ref={hostRef} className="term-host" />
        {status === 'ended' && (
          <div className="term-overlay">
            <p>Great practicing! 🎉</p>
            <button className="btn btn-kid" onClick={() => setReconnectNonce((n) => n + 1)}>
              Play again
            </button>
          </div>
        )}
        {status === 'error' && (
          <div className="term-overlay">
            <p>{message || 'Something went wrong.'}</p>
            <button className="btn btn-kid" onClick={() => setReconnectNonce((n) => n + 1)}>
              Try again
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
