import { useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, LoaderCircle, Network, RefreshCw, Stethoscope } from 'lucide-react'
import { StatusBadge } from '../components/StatusBadge'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { CheckStatus, DoctorReport } from '../types'
import { readableError } from '../utils'

type Filter = 'all' | CheckStatus

export function DoctorPage() {
  const initial = useAppStore((state) => state.snapshot?.doctor)
  const [report, setReport] = useState<DoctorReport | undefined>(initial)
  const [network, setNetwork] = useState(false)
  const [filter, setFilter] = useState<Filter>('all')
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const items = useMemo(() => report?.items.filter((item) => filter === 'all' || item.status === filter) || [], [filter, report])

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
      <div className="segmented filter-tabs" aria-label="诊断筛选">{(['all', 'error', 'warn', 'ok'] as Filter[]).map((item) => <button type="button" key={item} className={filter === item ? 'active' : ''} onClick={() => setFilter(item)}>{({ all: '全部', error: '错误', warn: '警告', ok: '正常' } as Record<Filter, string>)[item]}</button>)}</div>
      <section className="check-list" aria-label="诊断项">{items.map((item, index) => { const isOpen = expanded.has(index); return <article key={`${item.code}-${index}`} className={`check-row check-${item.status}`}><StatusBadge status={item.status} /><div><strong>{item.message}</strong>{item.suggest && <p>{item.suggest}</p>}{isOpen && item.details?.length && <ul>{item.details.map((detail) => <li key={detail}>{detail}</li>)}</ul>}</div>{Boolean(item.details?.length) && <button className="icon-button" type="button" aria-label={isOpen ? '收起详情' : '展开详情'} onClick={() => setExpanded((values) => { const next = new Set(values); next.has(index) ? next.delete(index) : next.add(index); return next })}>{isOpen ? <ChevronUp /> : <ChevronDown />}</button>}</article> })}</section>
    </div>
  )
}
