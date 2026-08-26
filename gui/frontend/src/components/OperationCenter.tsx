import { Check, CircleAlert, Clock3, ExternalLink, ListTodo, LoaderCircle, Trash2, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'
import { api } from '../lib/api'
import { operationLabel, operationRoute } from '../lib/operationRecord'
import { useAppStore } from '../store/app'
import type { OperationItem, OperationRecord } from '../types'
import { formatBytes } from '../utils'

interface OperationCenterProps {
  open: boolean
  onClose: () => void
}

export function OperationButton({ onClick }: { onClick: () => void }) {
  const running = useAppStore((state) => Object.values(state.operations).filter((item) => item.status === 'running').length)
  return <button className="titlebar-tool" type="button" aria-label={running ? `操作中心，${running} 项任务进行中` : '操作中心'} title="操作中心" onClick={onClick}><ListTodo />{running > 0 && <i>{running}</i>}</button>
}

export function OperationCenter({ open, onClose }: OperationCenterProps) {
  const operations = useAppStore((state) => state.operations)
  const clearOperation = useAppStore((state) => state.clearOperation)
  if (!open) return null
  const ordered = Object.values(operations).sort((left, right) => right.started_at.localeCompare(left.started_at))
  const running = ordered.filter((item) => item.status === 'running')
  const recent = ordered.filter((item) => item.status !== 'running')
  return <div className="panel-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="side-panel operation-center" role="dialog" aria-modal="true" aria-label="操作中心">
      <header className="side-panel-header"><div><strong>操作中心</strong><small>{running.length ? `${running.length} 项任务进行中` : '没有进行中的任务'}</small></div><button className="icon-button" type="button" aria-label="关闭操作中心" title="关闭" onClick={onClose}><X /></button></header>
      <div className="operation-center-content">
        <OperationSection title="进行中" empty="没有进行中的任务">{running.map((item) => <OperationCard key={item.id} item={item} onNavigate={onClose} />)}</OperationSection>
        <OperationSection title="最近完成" empty="当前窗口还没有完成记录" action={recent.length > 0 ? <button className="button text" type="button" onClick={() => recent.forEach((item) => clearOperation(item.id))}><Trash2 />清除全部</button> : undefined}>{recent.map((item) => <OperationCard key={item.id} item={item} onNavigate={onClose} />)}</OperationSection>
      </div>
    </aside>
  </div>
}

function OperationSection({ action, children, empty, title }: { action?: ReactNode; children: ReactNode[]; empty: string; title: string }) {
  return <section className="operation-section"><header><h2>{title}</h2>{action}</header>{children.length > 0 ? children : <div className="empty-inline">{empty}</div>}</section>
}

function OperationCard({ item, onNavigate }: { item: OperationRecord; onNavigate: () => void }) {
  const clearOperation = useAppStore((state) => state.clearOperation)
  const files = downloadFiles(item.items)
  const progress = aggregateProgress(files)
  return <article className={`operation-card operation-card-${item.status}`}>
    <header><span className="operation-status-icon">{item.status === 'running' ? <LoaderCircle className="spin" /> : item.status === 'complete' ? <Check /> : <CircleAlert />}</span><div><strong>{operationLabel(item.operation)}{progress !== undefined && item.status === 'running' ? ` ${progress}%` : ''}</strong><small>{item.status === 'running' ? item.summary || item.items.at(-1)?.stage || '处理中' : item.status === 'canceled' ? '已取消' : item.status === 'failed' ? '操作失败' : '已完成'}</small></div>{item.status === 'running' ? <button className="icon-button" type="button" aria-label={`取消${operationLabel(item.operation)}`} title="取消任务" onClick={() => void api.Cancel(item.id)}><X /></button> : <button className="icon-button" type="button" aria-label={`清除${operationLabel(item.operation)}`} title="清除" onClick={() => clearOperation(item.id)}><Trash2 /></button>}</header>
    {files.length > 0 && <div className="operation-files">{files.map((file) => <OperationFile key={file.key} item={file} />)}</div>}
    {item.error && <p className="operation-error">{item.error}</p>}
    {item.result_summary.length > 0 && <ul className="operation-result">{item.result_summary.map((line) => <li key={line}>{line}</li>)}</ul>}
    {item.status !== 'running' && <footer><span><Clock3 />{formatFinishedAt(item.finished_at)}</span>{item.status === 'failed' && <NavLink to={operationRoute(item.operation)} onClick={onNavigate}>返回来源页面<ExternalLink /></NavLink>}</footer>}
  </article>
}

function OperationFile({ item }: { item: OperationItem }) {
  const downloaded = item.bytes_downloaded || 0
  const total = item.total_bytes || 0
  return <div className="operation-file"><div><span title={item.filename}>{item.filename || item.version}</span><small>{item.source || item.version}</small></div><b>{total > 0 ? `${formatBytes(downloaded)} / ${formatBytes(total)}` : downloaded > 0 ? `${formatBytes(downloaded)} / 大小未知` : '大小未知'}</b><progress aria-label={`${item.filename || item.version} 下载进度`} value={total > 0 ? Math.min(downloaded, total) : undefined} max={total > 0 ? total : undefined} /></div>
}

export function downloadFiles(items: OperationItem[]) {
  return items.filter((item) => Boolean(item.filename))
}

export function aggregateProgress(files: OperationItem[]): number | undefined {
  if (files.length === 0 || files.some((item) => !item.total_bytes || item.total_bytes <= 0)) return undefined
  const total = files.reduce((sum, item) => sum + (item.total_bytes || 0), 0)
  const downloaded = files.reduce((sum, item) => sum + Math.min(item.bytes_downloaded || 0, item.total_bytes || 0), 0)
  return total > 0 ? Math.min(100, Math.round(downloaded * 100 / total)) : undefined
}

function formatFinishedAt(value?: string) {
  if (!value) return '完成时间未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
