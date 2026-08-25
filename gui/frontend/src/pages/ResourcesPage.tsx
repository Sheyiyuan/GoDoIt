import { ArchiveX, Check, CircleOff, DownloadCloud, Package, RefreshCw, Server, Waypoints } from 'lucide-react'
import { useParams } from 'react-router-dom'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import { formatBytes, readableError } from '../utils'

export function ResourcesPage() {
  const { kind = 'engines' } = useParams()
  const snapshot = useAppStore((state) => state.snapshot)
  const load = useAppStore((state) => state.load)
  const notify = useAppStore((state) => state.notify)
  const openModal = useAppStore((state) => state.openModal)
  if (!snapshot) return null
  const { assets, instances } = snapshot

  const refsForEngine = (id: string) => instances.filter((item) => item.engine === id).map((item) => item.name)
  const refsForSDK = (version: string) => instances.filter((item) => item.sdk === version).map((item) => item.name)
  const reportError = (error: unknown) => notify(readableError(error))

  const sourceAction = async (name: string, action: 'default' | 'toggle', disabled = false) => {
    try {
      if (action === 'default') await api.SetDefaultSource(name)
      else await api.SetSourceDisabled(name, disabled)
      await load()
      notify('来源设置已更新')
    } catch (error) { reportError(error) }
  }

  return (
    <div className="page resources-page">
      <header className="page-header"><div><p className="eyebrow">资源管理</p><h1>{resourceTitle(kind)}</h1></div><button className="button secondary" type="button" onClick={() => void load()}><RefreshCw />刷新</button></header>
      {kind === 'engines' && <ResourceTable headings={['版本', 'Edition', '来源', '平台', '引用条目', '状态']}>{assets.engines.map((engine) => { const refs = refsForEngine(engine.id); return <div className="resource-row" key={engine.id}><strong data-label="版本">{engine.version}</strong><span data-label="Edition">{engine.edition}</span><span data-label="来源">{engine.source}</span><span data-label="平台">{engine.target.os}/{engine.target.arch}</span><span data-label="引用条目">{refs.join('、') || '无引用'}</span><span data-label="状态" className={refs.length ? 'resource-ok' : 'resource-warn'}>{refs.length ? <><Check />使用中</> : <><CircleOff />孤儿</>}</span></div> })}</ResourceTable>}
      {kind === 'sdks' && <ResourceTable headings={['版本', '类型', '位置', '引用条目', '状态']}>{assets.sdks.map((sdk) => { const refs = refsForSDK(sdk.version); return <div className="resource-row five" key={`${sdk.kind}-${sdk.version}`}><strong data-label="版本">{sdk.version}</strong><span data-label="类型">{sdk.kind}</span><code data-label="位置" title={sdk.path}>{sdk.path}</code><span data-label="引用条目">{refs.join('、') || '无引用'}</span><span data-label="状态" className={refs.length || sdk.kind === 'system' ? 'resource-ok' : 'resource-warn'}>{refs.length || sdk.kind === 'system' ? <><Check />可用</> : <><CircleOff />孤儿</>}</span></div> })}</ResourceTable>}
      {kind === 'sources' && <section className="source-list">{assets.sources.map((source, index) => <article key={source.name}><span className="round-icon"><Waypoints /></span><div><strong>{source.name}</strong><small>{source.kind} · 优先级 {index + 1}</small></div>{index === 0 && !source.disabled && <span className="tag">默认</span>}<label className="toggle"><input type="checkbox" checked={!source.disabled} onChange={(event) => void sourceAction(source.name, 'toggle', !event.target.checked)} /><span />{source.disabled ? '已禁用' : '已启用'}</label>{index !== 0 && !source.disabled && <button className="button secondary" type="button" onClick={() => void sourceAction(source.name, 'default')}>设为默认</button>}</article>)}</section>}
      {kind === 'cache' && <><section className="orphan-summary"><span className="summary-mark"><ArchiveX /></span><div><strong>{assets.orphans.length} 个孤儿资产</strong><small>共 {formatBytes(assets.orphans.reduce((sum, item) => sum + item.size, 0))}</small></div><button className="button danger" type="button" disabled={!assets.orphans.length} onClick={() => openModal({ title: '清理孤儿资产', body: `将重新扫描引用并删除复查时仍无引用的 ${assets.orphans.length} 个资产。`, confirmLabel: '复查并清理', tone: 'danger', onConfirm: async () => { try { await runOperation(api.AutoRemove()); await load(); notify('孤儿资产已清理') } catch (error) { reportError(error) } } })}><ArchiveX />Auto remove</button></section><ResourceTable headings={['类型', '资产', '占用', '位置', '状态']}>{assets.orphans.map((item) => <div className="resource-row five" key={`${item.kind}-${item.id}`}><span data-label="类型">{item.kind}</span><strong data-label="资产">{item.id}</strong><span data-label="占用">{formatBytes(item.size)}</span><code data-label="位置" title={item.path}>{item.path}</code><span data-label="状态" className="resource-warn"><CircleOff />无引用</span></div>)}</ResourceTable></>}
    </div>
  )
}

function ResourceTable({ headings, children }: { headings: string[]; children: React.ReactNode }) {
  return <section className="resource-table"><div className={`resource-head ${headings.length === 5 ? 'five' : ''}`}>{headings.map((heading) => <span key={heading}>{heading}</span>)}</div>{children}</section>
}

function resourceTitle(kind: string) {
  return ({ engines: 'Engines', sdks: '.NET SDK', sources: '下载来源', cache: '缓存与孤儿' } as Record<string, string>)[kind] || '资源'
}

export function ResourceEmptyIcon({ kind }: { kind: string }) {
  const Icon = kind === 'engines' ? Package : kind === 'sources' ? Server : DownloadCloud
  return <Icon />
}
