import { useEffect, useMemo, useState } from 'react'
import { Download, Eye, EyeOff, MoreHorizontal, PackageCheck, Play, RefreshCw, Settings2, Star, Trash2, Unlink } from 'lucide-react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { IconAvatar } from '../components/IconAvatar'
import { IconPicker } from '../components/IconPicker'
import { StatusBadge } from '../components/StatusBadge'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { IconStrategy, InstanceDetails } from '../types'
import { formatBytes, readableError } from '../utils'

export function InstancePage() {
  const { name } = useParams()
  const navigate = useNavigate()
  const snapshot = useAppStore((state) => state.snapshot)
  const load = useAppStore((state) => state.load)
  const notify = useAppStore((state) => state.notify)
  const openModal = useAppStore((state) => state.openModal)
  const [details, setDetails] = useState<InstanceDetails | null>(null)
  const [error, setError] = useState('')
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [showIcons, setShowIcons] = useState(false)

  useEffect(() => {
    if (name || !snapshot) return
    const selected = snapshot.current || snapshot.instances[0]
    if (selected) navigate(`/instances/${encodeURIComponent(selected.name)}`, { replace: true })
  }, [name, navigate, snapshot])

  useEffect(() => {
    if (!name) return
    let active = true
    setError('')
    setDetails(null)
    api.GetInstanceDetails(name).then((result) => active && setDetails(result)).catch((reason) => active && setError(readableError(reason)))
    return () => { active = false }
  }, [name, snapshot])

  const instance = details?.instance || snapshot?.instances.find((item) => item.name === name)
  const template = useMemo(() => details?.templates.find((item) => item.id === instance?.template), [details, instance])

  if (!snapshot?.instances.length) return <EmptyInstances />
  if (!instance) return <PageLoading error={error} />

  const execute = async (started: Promise<{ operation_id: string }>, message: string) => {
    try {
      await runOperation(started)
      await load()
      notify(message)
      return true
    } catch (reason) {
      setError(readableError(reason))
      return false
    }
  }

  const setIcon = async (icon: IconStrategy) => {
    let sourcePath = ''
    if (icon === 'custom') {
      sourcePath = await api.PickIconFile()
      if (!sourcePath) return
    }
    setShowIcons(false)
    await execute(api.SetInstanceIcon(instance.name, { icon, source_path: sourcePath || undefined, background: instance.icon_background || undefined }), '条目图标已更新')
  }

  const setIconBackground = async (background: string) => {
    await execute(api.SetInstanceIcon(instance.name, { icon: instance.icon, background: background || undefined }), background ? '图标背景色已更新' : '图标背景已恢复透明')
  }

  return (
    <div className="page instance-page">
      <header className="page-header instance-header">
        <div className="instance-title"><IconAvatar instance={instance} size="large" /><div><h1>{instance.name}</h1><p><span className="tag">{instance.current ? '当前' : '可用'}</span>{instance.icon_missing && <span className="tag warning">图标已回退</span>}Godot {instance.engine.replace(`-${instance.edition}`, '')} · {instance.edition === 'dotnet' ? '.NET edition' : 'standard edition'}</p></div></div>
        <div className="header-actions"><button className="button primary" type="button" onClick={async () => { try { await api.Launch(instance.name); notify(`已启动 ${instance.name}`) } catch (reason) { setError(readableError(reason)) } }}><Play />启动 Godot</button><button className="icon-button bordered" type="button" aria-label="条目操作" title="条目操作" onClick={() => setShowIcons((shown) => !shown)}><MoreHorizontal /></button></div>
        {showIcons && <div className="floating-picker"><strong>条目图标</strong><IconPicker value={instance.icon} background={instance.icon_background} onChange={(value) => void setIcon(value)} onBackgroundChange={(value) => void setIconBackground(value)} onPickCustom={() => void setIcon('custom')} /></div>}
      </header>
      {error && <div className="inline-error" role="alert">{error}<button type="button" onClick={() => setError('')}>关闭</button></div>}

      <section className="runtime-band" aria-labelledby="runtime-title">
        <h2 id="runtime-title">运行配置</h2>
        <dl>
          <div><dt>引擎</dt><dd>Godot {instance.engine.replace(`-${instance.edition}`, '')}</dd></div>
          <div><dt>SDK 策略</dt><dd>{instance.sdk_strategy ? `${instance.sdk_strategy}${instance.sdk ? ` · ${instance.sdk}` : ''}` : '无需 SDK'}</dd></div>
          <div><dt>导出模板</dt><dd>{instance.template || '未绑定'}</dd></div>
          <div><dt>状态</dt><dd><StatusBadge status={instance.template_missing ? 'warn' : 'ok'} label={instance.template_missing ? '导出资源缺失' : '可启动'} /></dd></div>
        </dl>
      </section>

      <section className="section-block" aria-labelledby="dependency-title">
        <div className="section-heading"><div><h2 id="dependency-title">条目资源</h2><p>导出模板</p></div></div>
        <div className="dependency-row">
          <span className="round-icon"><PackageCheck /></span>
          <div className="dependency-copy"><strong>{instance.template || instance.engine}</strong><small>{template ? `${template.source} · ${formatBytes(template.size)} · ${template.references.length} 个引用` : instance.template_missing ? '绑定存在，模板资产缺失' : '未绑定导出模板'}</small></div>
          {instance.template ? <button className="button secondary" type="button" onClick={() => openModal({ title: '解除模板绑定', body: `将从 ${instance.name} 解除 ${instance.template}。模板资产不会立即删除。`, confirmLabel: '解除绑定', onConfirm: async () => { await execute(api.DetachTemplate(instance.name), '模板绑定已解除') } })}><Unlink />解除绑定</button> : <button className="button primary" type="button" onClick={() => void execute(api.AttachTemplate(instance.name, ''), '模板已安装并绑定')}><Download />下载模板</button>}
        </div>
      </section>

      <section className="section-block environment-section" aria-labelledby="env-title">
        <div className="section-heading"><div><h2 id="env-title">运行环境</h2><p>{details?.env.vars.length || 0} 个派生变量</p></div><Link to="/settings">管理环境</Link></div>
        {details?.env_error && <div className="environment-warning">{details.env_error}</div>}
        <div className="env-table">
          {details?.env.vars.length ? details.env.vars.map((variable) => <div className="env-row" key={variable.key}><code>{variable.key}</code><span>{revealed.has(variable.key) ? variable.value : '••••••••••••'}</span><small>{variable.origin}</small><button className="icon-button" type="button" aria-label={`${revealed.has(variable.key) ? '隐藏' : '显示'} ${variable.key}`} onClick={() => setRevealed((keys) => { const next = new Set(keys); next.has(variable.key) ? next.delete(variable.key) : next.add(variable.key); return next })}>{revealed.has(variable.key) ? <EyeOff /> : <Eye />}</button></div>) : <div className="empty-row">此条目没有额外注入变量</div>}
        </div>
      </section>

      <section className="action-footer">
        <div><Settings2 /><span><strong>条目操作</strong><small>配置变更会立即写入当前 gdit 根目录</small></span></div>
        <div>
          {!instance.current && <button className="button secondary" type="button" onClick={async () => { try { await api.SetDefault(instance.name); await load(); notify(`${instance.name} 已设为当前`) } catch (reason) { setError(readableError(reason)) } }}><Star />设为当前</button>}
          <button className="button danger-quiet" type="button" disabled={instance.current} title={instance.current ? '当前条目不能卸载' : '卸载条目'} onClick={() => openModal({ title: `卸载 ${instance.name}`, body: '条目文件和自定义图标将被删除。引擎、SDK 与模板会保留为孤儿资产，稍后可复查清理。', confirmLabel: '卸载条目', tone: 'danger', onConfirm: async () => { if (await execute(api.RemoveInstance(instance.name), `${instance.name} 已卸载`)) navigate('/instances') } })}><Trash2 />卸载</button>
        </div>
      </section>
    </div>
  )
}

function EmptyInstances() {
  return <div className="empty-page"><span><RefreshCw /></span><h1>尚无条目</h1><Link className="button primary" to="/instances/new">新建条目</Link></div>
}

function PageLoading({ error }: { error: string }) {
  return <div className="empty-page"><span><RefreshCw className={!error ? 'spin' : ''} /></span><h1>{error || '正在读取条目'}</h1></div>
}
