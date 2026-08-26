import { useMemo, useState } from 'react'
import { AlertTriangle, CheckCircle2, ChevronDown, ChevronUp, CircleAlert, Info, LoaderCircle, Network, RefreshCw, Stethoscope } from 'lucide-react'
import { StatusBadge } from '../components/StatusBadge'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { DoctorReport } from '../types'
import { readableError } from '../utils'
import { diagnosticGroupLabels, doctorDiagnostics, type DiagnosticGroup } from '../lib/diagnostics'

const issueGroups: DiagnosticGroup[] = ['failure', 'attention', 'integration']

export function DoctorPage() {
  const initial = useAppStore((state) => state.snapshot?.doctor)
  const [report, setReport] = useState<DoctorReport | undefined>(initial)
  const [network, setNetwork] = useState(false)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [integrationOpen, setIntegrationOpen] = useState(false)
  const [normalOpen, setNormalOpen] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const dismissed = useAppStore((state) => state.dismissedWarnings)
  const dismissWarning = useAppStore((state) => state.dismissWarning)
  const issues = useMemo(() => report ? doctorDiagnostics(report) : [], [report])
  const normal = useMemo(() => report?.items.filter((item) => item.status === 'ok') || [], [report])

  const run = async () => {
    setRunning(true)
    setError('')
    try {
      setReport(await runOperation<DoctorReport>(api.GetDoctor(network)))
    } catch (reason) {
      setError(readableError(reason))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="page doctor-page">
      <header className="page-header"><div><p className="eyebrow">环境诊断</p><h1>Doctor</h1></div><div className="header-actions"><label className="toggle"><input type="checkbox" checked={network} onChange={(event) => setNetwork(event.target.checked)} /><span /><Network />检查来源可达性</label><button className="button primary" type="button" disabled={running} onClick={() => void run()}>{running ? <LoaderCircle className="spin" /> : <RefreshCw />}{running ? '检查中' : '重新检查'}</button></div></header>
      {error && <div className="inline-error" role="alert">{error}</div>}
      <section className="doctor-summary"><div className="summary-mark"><Stethoscope /></div><div><strong>{report?.error_count ? '发现需要处理的问题' : report?.warn_count ? '可以使用，有少量提醒' : '环境状态正常'}</strong><small>{report?.root}</small></div><dl><div><dt>正常</dt><dd>{report?.ok_count || 0}</dd></div><div><dt>警告</dt><dd>{report?.warn_count || 0}</dd></div><div><dt>错误</dt><dd>{report?.error_count || 0}</dd></div></dl></section>
      <section className="doctor-groups" aria-label="诊断项">{issueGroups.map((group) => {
        const items = issues.filter((item) => item.group === group && !dismissed[item.id])
        if (items.length === 0) return null
        const collapsed = group === 'integration' && !integrationOpen
        return <div className={`doctor-group doctor-group-${group}`} key={group}>
          <button className="doctor-group-heading" type="button" disabled={group !== 'integration'} onClick={() => setIntegrationOpen((value) => !value)}><span>{group === 'failure' ? <CircleAlert /> : group === 'attention' ? <AlertTriangle /> : <Info />}<strong>{diagnosticGroupLabels[group]}</strong><i>{items.length}</i></span>{group === 'integration' && (collapsed ? <ChevronDown /> : <ChevronUp />)}</button>
          {!collapsed && <div className="check-list">{items.map((item) => { const isOpen = expanded.has(item.id); return <article key={item.id} className={`check-row check-${item.status}`}><StatusBadge status={item.status} /><div><strong>{item.message}</strong>{item.suggest && <p>{item.suggest}</p>}{isOpen && item.details?.length ? <ul>{item.details.map((detail) => <li key={detail}>{detail}</li>)}</ul> : null}</div><div className="check-actions">{item.status === 'warn' && group !== 'integration' && <button className="button text" type="button" onClick={() => dismissWarning(item.id)}>本次关闭</button>}{Boolean(item.details?.length) && <button className="icon-button" type="button" aria-label={isOpen ? '收起详情' : '展开详情'} onClick={() => setExpanded((values) => { const next = new Set(values); next.has(item.id) ? next.delete(item.id) : next.add(item.id); return next })}>{isOpen ? <ChevronUp /> : <ChevronDown />}</button>}</div></article> })}</div>}
        </div>
      })}
      <div className="doctor-group doctor-group-ok"><button className="doctor-group-heading" type="button" onClick={() => setNormalOpen((value) => !value)}><span><CheckCircle2 /><strong>正常</strong><i>{normal.length}</i></span>{normalOpen ? <ChevronUp /> : <ChevronDown />}</button>{normalOpen && <div className="check-list">{normal.map((item, index) => <article key={`${item.code}-${index}`} className="check-row check-ok"><StatusBadge status="ok" /><div><strong>{item.message}</strong>{item.suggest && <p>{item.suggest}</p>}</div></article>)}</div>}</div>
      </section>
    </div>
  )
}
