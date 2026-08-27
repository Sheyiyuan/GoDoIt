import { ListTodo, LoaderCircle, X } from 'lucide-react'
import { api } from '../lib/api'
import { operationLabel } from '../lib/operationRecord'
import { useAppStore } from '../store/app'
import { aggregateProgress } from './OperationCenter'

export function OperationTray({ onOpen }: { onOpen: () => void }) {
  const operations = useAppStore((state) => state.operations)
  const running = Object.values(operations).filter((event) => event.status === 'running')
  if (running.length === 0) return null
  const current = running[0]
  const files = current.items.filter((item) => Boolean(item.filename))
  const progress = aggregateProgress(files)
  return (
    <aside className="operation-tray" aria-live="polite">
      <button className="operation-summary" type="button" onClick={onOpen}><ListTodo /><span><strong>{running.length > 1 ? `${operationLabel(current.operation)}等 ${running.length} 项任务` : operationLabel(current.operation)}{progress !== undefined ? ` ${progress}%` : ''}</strong><small>{current.summary || current.items.at(-1)?.stage || '处理中'}</small></span></button>
      <LoaderCircle className="spin" aria-hidden="true" />
      <button className="icon-button" type="button" title="取消任务" aria-label={`取消${operationLabel(current.operation)}`} onClick={() => void api.Cancel(current.id)}><X /></button>
    </aside>
  )
}
