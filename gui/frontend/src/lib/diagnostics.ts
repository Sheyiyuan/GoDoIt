import type { AppSnapshot, CheckResult, DoctorReport } from '../types'

export type DiagnosticGroup = 'failure' | 'attention' | 'integration'

export interface DiagnosticEntry extends CheckResult {
  id: string
  group: DiagnosticGroup
  bootstrap: boolean
}

export const diagnosticGroupLabels: Record<DiagnosticGroup, string> = {
  failure: '故障',
  attention: '需要注意',
  integration: '可选集成',
}

export function classifyDoctorItem(item: CheckResult): DiagnosticGroup | undefined {
  if (item.status === 'error') return 'failure'
  if (item.status !== 'warn') return undefined
  return item.code === 'shim' ? 'integration' : 'attention'
}

export function snapshotDiagnostics(snapshot: AppSnapshot | null): DiagnosticEntry[] {
  if (!snapshot) return []
  const bootstrap = (snapshot.issues || []).filter(Boolean).map((message, index) => ({
    id: `bootstrap:${index}:${message}`,
    code: 'bootstrap',
    status: 'error' as const,
    message,
    suggest: '重新读取；若问题仍然存在，请打开 Doctor 查看完整诊断。',
    group: 'failure' as const,
    bootstrap: true,
  }))
  return [...bootstrap, ...doctorDiagnostics(snapshot.doctor)]
}

export function doctorDiagnostics(report: DoctorReport): DiagnosticEntry[] {
  return report.items.flatMap((item) => {
    const group = classifyDoctorItem(item)
    return group ? [{ ...item, id: `doctor:${item.code}:${item.message}`, group, bootstrap: false }] : []
  })
}

export function visibleReminderCount(entries: DiagnosticEntry[], dismissed: Record<string, true>) {
  return entries.filter((item) => item.group !== 'integration' && !dismissed[item.id]).length
}
