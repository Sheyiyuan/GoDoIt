import { useCallback, useEffect, useMemo, useState } from 'react'
import { Clipboard, Eye, EyeOff, Pencil, Plus, Save, Trash2, X } from 'lucide-react'
import { api } from '../lib/api'
import { useAppStore } from '../store/app'
import type { ConfiguredEnvVar, EnvironmentDetails, EnvScope } from '../types'
import { readableError } from '../utils'

interface EnvironmentEditorProps {
  name: string
  editableScope: Extract<EnvScope, 'global' | 'instance'>
  onChanged?: () => void
}

interface EnvironmentItem {
  id: string
  key: string
  value: string
  label: string
  sensitive: boolean
  editable: boolean
  configured?: ConfiguredEnvVar
}

export function EnvironmentEditor({ name, editableScope, onChanged }: EnvironmentEditorProps) {
  const notify = useAppStore((state) => state.notify)
  const openModal = useAppStore((state) => state.openModal)
  const [details, setDetails] = useState<EnvironmentDetails | null>(null)
  const [selectedID, setSelectedID] = useState('')
  const [editing, setEditing] = useState<'new' | 'edit' | ''>('')
  const [draftKey, setDraftKey] = useState('')
  const [draftValue, setDraftValue] = useState('')
  const [revealed, setRevealed] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setDetails(await api.GetEnvironment(name))
    } catch (reason) {
      setError(readableError(reason))
    } finally {
      setLoading(false)
    }
  }, [name])

  useEffect(() => { void load() }, [load])

  const configured = useMemo<EnvironmentItem[]>(() => (details?.configured.vars || []).map((variable) => ({
    id: `configured:${variable.scope}:${variable.key}`,
    key: variable.key,
    value: variable.value,
    label: variable.scope === 'global' ? '全局配置' : variable.scope === 'platform' ? '当前平台' : '条目配置',
    sensitive: variable.sensitive,
    editable: variable.editable && variable.scope === editableScope,
    configured: variable,
  })), [details?.configured.vars, editableScope])
  const effective = useMemo<EnvironmentItem[]>(() => (details?.effective.vars || []).map((variable) => ({
    id: `effective:${variable.key}`,
    key: variable.key,
    value: variable.value,
    label: `最终值 · ${variable.origin}`,
    sensitive: variable.sensitive,
    editable: false,
  })), [details?.effective.vars])
  const items = useMemo(() => [...configured, ...effective], [configured, effective])
  const selected = items.find((item) => item.id === selectedID) || items[0]

  useEffect(() => {
    if (!items.length) {
      setSelectedID('')
      return
    }
    if (!items.some((item) => item.id === selectedID)) setSelectedID(items[0].id)
  }, [items, selectedID])

  const select = (id: string) => {
    setSelectedID(id)
    setEditing('')
    setRevealed(false)
    setError('')
  }

  const beginNew = () => {
    setEditing('new')
    setDraftKey('')
    setDraftValue('')
    setRevealed(false)
  }

  const beginEdit = () => {
    if (!selected?.editable) return
    setEditing('edit')
    setDraftKey(selected.key)
    setDraftValue(selected.value)
    setRevealed(false)
  }

  const save = async () => {
    const key = draftKey.trim()
    if (!key) return
    setError('')
    try {
      await api.SetEnvVar(editableScope === 'instance' ? name : '', key, draftValue)
      await load()
      setSelectedID(`configured:${editableScope}:${key}`)
      setEditing('')
      setDraftKey('')
      setDraftValue('')
      onChanged?.()
      notify(`${key} 已保存`)
    } catch (reason) {
      setError(readableError(reason))
    }
  }

  const remove = () => {
    if (!selected?.editable) return
    openModal({
      title: `删除 ${selected.key}`,
      body: `将从${editableScope === 'instance' ? `条目 ${name}` : '全局配置'}中删除该环境变量。最终启动环境会立即重新计算。`,
      confirmLabel: '删除变量',
      tone: 'danger',
      onConfirm: async () => {
        try {
          await api.UnsetEnvVar(editableScope === 'instance' ? name : '', selected.key)
          setSelectedID('')
          await load()
          onChanged?.()
          notify(`${selected.key} 已删除`)
        } catch (reason) {
          setError(readableError(reason))
        }
      },
    })
  }

  const copyValue = async () => {
    if (!selected) return
    try {
      await navigator.clipboard.writeText(selected.value)
      notify(`${selected.key} 已复制`)
    } catch (reason) {
      setError(readableError(reason))
    }
  }

  return (
    <div className="environment-editor">
      <div className="environment-editor-toolbar"><span>{editableScope === 'instance' ? '条目环境' : '全局环境'}</span><button className="button secondary" type="button" onClick={beginNew}><Plus />新增变量</button></div>
      {error && <div className="inline-error" role="alert">{error}<button type="button" onClick={() => setError('')}>关闭</button></div>}
      {details?.effective_error && <div className="environment-warning">最终环境暂不可计算：{details.effective_error}</div>}
      {loading && !details ? <div className="empty-inline">正在读取环境变量</div> : <div className="environment-editor-content">
        <div className="environment-variable-list" aria-label="环境变量列表">
          <EnvironmentGroup title="配置层" items={configured} selectedID={selected?.id} onSelect={select} />
          <EnvironmentGroup title="最终有效值" items={effective} selectedID={selected?.id} onSelect={select} />
        </div>
        <div className="environment-variable-detail">
          {editing ? <>
            <header><div><strong>{editing === 'new' ? '新增环境变量' : `编辑 ${draftKey}`}</strong><small>{editableScope === 'instance' ? `条目 ${name}` : '全局配置'}</small></div><button className="icon-button" type="button" aria-label="取消编辑" onClick={() => setEditing('')}><X /></button></header>
            <label><span>变量名</span><input autoFocus disabled={editing === 'edit'} value={draftKey} onChange={(event) => setDraftKey(event.target.value)} /></label>
            <label><span>变量值</span><textarea value={draftValue} onChange={(event) => setDraftValue(event.target.value)} /></label>
            <footer><button className="button primary" type="button" disabled={!draftKey.trim()} onClick={() => void save()}><Save />保存</button></footer>
          </> : selected ? <>
            <header><div><strong>{selected.key}</strong><small>{selected.label}{selected.editable ? ' · 可编辑' : ' · 只读'}</small></div><div>{selected.sensitive && <button className="icon-button" type="button" aria-label={revealed ? `隐藏 ${selected.key}` : `显示 ${selected.key}`} title={revealed ? '隐藏值' : '显示值'} onClick={() => setRevealed((value) => !value)}>{revealed ? <EyeOff /> : <Eye />}</button>}<button className="icon-button" type="button" aria-label={`复制 ${selected.key}`} title={selected.sensitive && !revealed ? '显示后可复制' : '复制值'} disabled={selected.sensitive && !revealed} onClick={() => void copyValue()}><Clipboard /></button></div></header>
            <label><span>完整值</span><textarea readOnly value={selected.sensitive && !revealed ? '••••••••••••' : selected.value} /></label>
            {selected.editable && <footer><button className="button secondary" type="button" onClick={beginEdit}><Pencil />编辑</button><button className="button danger-quiet" type="button" onClick={remove}><Trash2 />删除</button></footer>}
          </> : <div className="empty-inline">尚无环境变量</div>}
        </div>
      </div>}
    </div>
  )
}

function EnvironmentGroup({ title, items, selectedID, onSelect }: { title: string; items: EnvironmentItem[]; selectedID?: string; onSelect: (id: string) => void }) {
  return <section><h3>{title}</h3>{items.length ? items.map((item) => <button type="button" key={item.id} className={item.id === selectedID ? 'active' : ''} onClick={() => onSelect(item.id)}><code>{item.key}</code><small>{item.label}</small></button>) : <p>无</p>}</section>
}
