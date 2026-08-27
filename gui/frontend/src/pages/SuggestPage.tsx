import { useState } from 'react'
import { AlertTriangle, CheckCircle2, FileCode2, FolderOpen, LoaderCircle, PackagePlus, Search } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { ProjectSuggestion } from '../types'
import { readableError } from '../utils'

export function SuggestPage() {
  const navigate = useNavigate()
  const load = useAppStore((state) => state.load)
  const notify = useAppStore((state) => state.notify)
  const [projectDir, setProjectDir] = useState('')
  const [suggestion, setSuggestion] = useState<ProjectSuggestion | null>(null)
  const [analysing, setAnalysing] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [name, setName] = useState('project-workspace')
  const [includeTemplate, setIncludeTemplate] = useState(true)
  const [setCurrent, setSetCurrent] = useState(false)

  const choose = async () => {
    const selected = await api.PickProjectDirectory()
    if (selected) { setProjectDir(selected); setSuggestion(null) }
  }

  const analyse = async () => {
    if (!projectDir) return
    setAnalysing(true)
    setError('')
    try {
      setSuggestion(await runOperation<ProjectSuggestion>(api.Suggest(projectDir)))
    } catch (reason) {
      setError(readableError(reason))
    } finally {
      setAnalysing(false)
    }
  }

  const install = async () => {
    if (!suggestion) return
    setInstalling(true)
    setError('')
    try {
      await runOperation(api.InstallSuggestion({ project_dir: projectDir, name, set_current: setCurrent, include_template: includeTemplate }))
      await load()
      notify(`${name} 已按项目建议安装`)
      navigate(`/instances/${encodeURIComponent(name)}`)
    } catch (reason) {
      setError(readableError(reason))
    } finally {
      setInstalling(false)
    }
  }

  return (
    <div className="page suggest-page">
      <header className="page-header"><div><p className="eyebrow">只读分析</p><h1>项目建议</h1></div></header>
      <div className="page-body">
        {error && <div className="inline-error" role="alert">{error}</div>}
        <section className="path-selector"><div><FolderOpen /><span><strong>{projectDir || '选择 Godot 项目目录'}</strong><small>路径不会写入项目或保存到配置</small></span></div><button className="button secondary" type="button" onClick={() => void choose()}>选择目录</button><button className="button primary" type="button" disabled={!projectDir || analysing} onClick={() => void analyse()}>{analysing ? <LoaderCircle className="spin" /> : <Search />}{analysing ? '分析中' : '分析项目'}</button></section>

        {suggestion && <>
        <section className="suggest-summary"><div><span>引擎系列</span><strong>{suggestion.engine_series}</strong></div><div><span>Edition</span><strong>{suggestion.edition}</strong></div><div><span>SDK</span><strong>{suggestion.sdk_version || suggestion.sdk_channel || '无需 SDK'}</strong></div><div><span>可安装</span><strong>{suggestion.installable ? '是' : '否'}</strong></div></section>
        <section className="section-block"><div className="section-heading"><div><h2>分析证据</h2><p>{suggestion.evidence.length} 项</p></div></div><div className="evidence-list">{suggestion.evidence.map((evidence) => <div key={`${evidence.kind}-${evidence.path}`}><FileCode2 /><span><strong>{evidence.kind}</strong><small>{evidence.path}</small></span><code>{evidence.value}</code></div>)}</div></section>
        {suggestion.diagnostics.length > 0 && <section className="diagnostic-band"><h2>诊断</h2>{suggestion.diagnostics.map((diagnostic) => <div className={diagnostic.level} key={diagnostic.code}>{diagnostic.level === 'error' ? <AlertTriangle /> : <CheckCircle2 />}<span><strong>{diagnostic.message}</strong>{diagnostic.path && <small>{diagnostic.path}</small>}</span></div>)}</section>}
        <section className="section-block suggest-review"><div className="section-heading"><div><h2>安装建议</h2><p>{suggestion.diagnostics.filter((item) => item.level === 'warning').length} 项 warning 将随安装保留</p></div></div><div className="form-grid"><label><span>条目名</span><input value={name} onChange={(event) => setName(event.target.value)} /></label><label className="toggle-field"><span>安装导出模板</span><label className="toggle"><input type="checkbox" checked={includeTemplate} onChange={(event) => setIncludeTemplate(event.target.checked)} /><span />{includeTemplate ? '安装并绑定' : '跳过模板'}</label></label></div><label className="toggle standalone"><input type="checkbox" checked={setCurrent} onChange={(event) => setSetCurrent(event.target.checked)} /><span />安装后设为当前条目</label><button className="button primary" type="button" disabled={!suggestion.installable || !name || installing} onClick={() => void install()}>{installing ? <LoaderCircle className="spin" /> : <PackagePlus />}{installing ? '安装中' : '按建议安装'}</button></section>
        </>}
      </div>
    </div>
  )
}
