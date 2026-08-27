import { AlertTriangle, CheckCircle2, CircleAlert } from 'lucide-react'
import type { CheckStatus } from '../types'

export function StatusBadge({ status, label }: { status: CheckStatus; label?: string }) {
  const Icon = status === 'ok' ? CheckCircle2 : status === 'warn' ? AlertTriangle : CircleAlert
  return <span className={`status-badge status-${status}`}><Icon aria-hidden="true" />{label || ({ ok: '正常', warn: '警告', error: '错误' }[status])}</span>
}
