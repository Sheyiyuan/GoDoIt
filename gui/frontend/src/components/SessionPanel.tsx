import { useEffect, useState } from 'react'
import { Clock3, ExternalLink, LoaderCircle, MonitorPlay, OctagonX, Power, X } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { useAppStore } from '../store/app'
import type { SessionInfo } from '../types'

export function SessionButton({ onClick }: { onClick: () => void }) {
  const count = useAppStore((state) => state.sessions.length)
  return <button className={`session-button ${count ? 'active' : ''}`} type="button" aria-label={`运行中 ${count}`} title="运行会话" onClick={onClick}><MonitorPlay /><span>运行中 {count}</span></button>
}

export function SessionPanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const sessions = useAppStore((state) => state.sessions)
  const error = useAppStore((state) => state.sessionError)
  if (!open) return null
  return <div className="panel-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <aside className="side-panel session-panel" role="dialog" aria-modal="true" aria-label="运行会话">
      <header className="side-panel-header"><div><strong>运行会话</strong><small>{sessions.length ? `${sessions.length} 个由 GoDoIt GUI 启动的进程` : '当前没有登记的进程'}</small></div><button className="icon-button" type="button" aria-label="关闭运行会话" title="关闭" onClick={onClose}><X /></button></header>
      {error && <div className="inline-error" role="alert">{error}</div>}
      <div className="session-panel-content">{sessions.length ? <SessionList sessions={sessions} onNavigate={onClose} /> : <div className="empty-inline">当前没有运行会话</div>}</div>
    </aside>
  </div>
}

export function SessionList({ sessions, onError, onNavigate }: { sessions: SessionInfo[]; onError?: (message: string) => void; onNavigate?: () => void }) {
  const requestStop = useAppStore((state) => state.requestStopSession)
  const forceStop = useAppStore((state) => state.forceStopSession)
  const openModal = useAppStore((state) => state.openModal)

  const report = (reason: unknown) => onError?.(reason instanceof Error ? reason.message : String(reason))
  const confirmForceStop = (session: SessionInfo) => openModal({
    title: `强制结束 ${session.instance_name}`,
    body: '强制结束会立即终止该 Godot 进程，未保存内容可能丢失。',
    confirmLabel: '强制结束',
    tone: 'danger',
    onConfirm: async () => { try { await forceStop(session.session_id) } catch (reason) { report(reason) } },
  })

  return <div className="session-list">{sessions.map((session) => <SessionRow key={session.session_id} session={session} onNavigate={onNavigate} onRequestStop={() => void requestStop(session.session_id).catch(report)} onForceStop={() => confirmForceStop(session)} />)}</div>
}

function SessionRow({ session, onForceStop, onNavigate, onRequestStop }: { session: SessionInfo; onForceStop: () => void; onNavigate?: () => void; onRequestStop: () => void }) {
  const [forceAvailable, setForceAvailable] = useState(false)
  useEffect(() => {
    if (session.status !== 'stopping') {
      setForceAvailable(false)
      return
    }
    const timer = window.setTimeout(() => setForceAvailable(true), 2000)
    return () => window.clearTimeout(timer)
  }, [session.status])

  return <article className="session-row">
    <MonitorPlay />
    <div className="session-copy"><NavLink to={`/instances/${encodeURIComponent(session.instance_name)}`} onClick={onNavigate}>{session.instance_name}<ExternalLink /></NavLink><small>{session.engine_id} · PID {session.pid}</small><small><Clock3 />{formatStartedAt(session.started_at)} · {session.status === 'stopping' ? '正在等待退出' : '运行中'}</small></div>
    {session.status === 'stopping'
      ? forceAvailable
        ? <button className="button danger-quiet" type="button" onClick={onForceStop}><OctagonX />强制结束</button>
        : <button className="button secondary" type="button" disabled><LoaderCircle className="spin" />等待退出</button>
      : <button className="button secondary" type="button" onClick={onRequestStop}><Power />关闭</button>}
  </article>
}

function formatStartedAt(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
