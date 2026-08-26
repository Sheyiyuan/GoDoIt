import { useEffect, useState } from 'react'
import { Eye, EyeOff, Pencil, Plus, Save, Trash2, Waypoints } from 'lucide-react'
import { api } from '../lib/api'
import { useAppStore } from '../store/app'
import type { ConfiguredEnvVar, EnvironmentDetails, TitlebarStyle } from '../types'
import { readableError } from '../utils'

export function SettingsPage() {
  const snapshot = useAppStore((state) => state.snapshot)
  const notify = useAppStore((state) => state.notify)
  const updateGUISettings = useAppStore((state) => state.updateGUISettings)
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState('')
  const [titlebarStyle, setTitlebarStyle] = useState<TitlebarStyle>('auto')
  const [environment, setEnvironment] = useState<EnvironmentDetails | null>(null)
  const [selected, setSelected] = useState<ConfiguredEnvVar | null>(null)

  useEffect(() => {
    if (snapshot?.gui) setTitlebarStyle(snapshot.gui.titlebar_style)
  }, [snapshot?.gui])

  const loadEnvironment = async () => {
    try {
      setEnvironment(await api.GetEnvironment(''))
    } catch (reason) {
      setError(readableError(reason))
    }
  }

  useEffect(() => { void loadEnvironment() }, [])

  const save = async () => {
    setError('')
    try {
      await api.SetEnvVar('', key, value)
      await loadEnvironment()
      setKey(''); setValue('')
      notify(`全局变量 ${key} 已保存`)
    } catch (reason) { setError(readableError(reason)) }
  }

  const editVariable = (variable: ConfiguredEnvVar) => {
    if (!variable.editable) return
    setSelected(variable)
    setKey(variable.key)
    setValue(variable.value)
    setVisible(false)
  }

  const removeVariable = async (variable: ConfiguredEnvVar) => {
    if (!variable.editable) return
    setError('')
    try {
      await api.UnsetEnvVar('', variable.key)
      if (selected?.key === variable.key) { setSelected(null); setKey(''); setValue('') }
      await loadEnvironment()
      notify(`全局变量 ${variable.key} 已删除`)
    } catch (reason) { setError(readableError(reason)) }
  }

  const updateTitlebarStyle = async (style: TitlebarStyle) => {
    const previous = titlebarStyle
    setTitlebarStyle(style)
    setError('')
    try {
      await updateGUISettings({ titlebar_style: style })
      notify('窗口顶栏风格已更新')
    } catch (reason) {
      setTitlebarStyle(previous)
      setError(readableError(reason))
    }
  }

  return (
    <div className="page settings-page">
      <header className="page-header"><div><p className="eyebrow">config.toml</p><h1>设置</h1></div></header>
      {error && <div className="inline-error">{error}</div>}
      <section className="settings-band"><div className="section-heading"><div><h2>数据位置</h2><p>当前 gdit 根目录</p></div></div><code className="root-path">{snapshot?.root}</code></section>
      <section className="settings-band"><div className="section-heading"><div><h2>窗口顶栏</h2><p>无边框窗口控制位置</p></div></div><div className="titlebar-style-setting"><div className="segmented" role="group" aria-label="窗口顶栏风格"><button className={titlebarStyle === 'auto' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('auto')}>跟随系统</button><button className={titlebarStyle === 'mac' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('mac')}>左上角红绿灯</button><button className={titlebarStyle === 'windows' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('windows')}>右上角窗口按钮</button></div></div></section>
      <section className="settings-band"><div className="section-heading"><div><h2>全局环境</h2><p>[environment] · {environment?.configured.vars.length || 0} 项配置</p></div></div><div className="env-editor"><label><span>变量名</span><input value={key} onChange={(event) => setKey(event.target.value.toUpperCase())} placeholder="VARIABLE_NAME" /></label><label><span>变量值</span><div className="input-with-action"><input type={visible ? 'text' : 'password'} value={value} onChange={(event) => setValue(event.target.value)} /><button className="icon-button" type="button" aria-label={visible ? '隐藏值' : '显示值'} onClick={() => setVisible((shown) => !shown)}>{visible ? <EyeOff /> : <Eye />}</button></div></label><button className="button primary" type="button" disabled={!key} onClick={() => void save()}><Save />{selected ? '更新变量' : '保存变量'}</button></div><div className="configured-env-list">{environment?.configured.vars.map((variable) => <div className="configured-env-row" key={`${variable.scope}:${variable.key}`}><code>{variable.key}</code><span>{variable.sensitive ? '••••••••••••' : variable.value}</span><small>{variable.scope === 'platform' ? '当前平台 · 只读' : variable.editable ? '全局 · 可编辑' : '只读'}</small><div>{variable.editable && <><button className="icon-button" type="button" aria-label={`编辑 ${variable.key}`} title={`编辑 ${variable.key}`} onClick={() => editVariable(variable)}><Pencil /></button><button className="icon-button" type="button" aria-label={`删除 ${variable.key}`} title={`删除 ${variable.key}`} onClick={() => void removeVariable(variable)}><Trash2 /></button></>}</div></div>)}</div></section>
      <section className="settings-band"><div className="section-heading"><div><h2>下载来源</h2><p>顺序与启禁用</p></div></div><div className="compact-source-list">{snapshot?.assets.sources.map((source, index) => <div key={source.name}><Waypoints /><span><strong>{source.name}</strong><small>{source.kind}</small></span><em>{source.disabled ? '禁用' : index === 0 ? '默认' : `优先级 ${index + 1}`}</em></div>)}</div><p className="config-note"><Plus />自定义来源 URL 继续在 config.toml 中维护</p></section>
    </div>
  )
}
