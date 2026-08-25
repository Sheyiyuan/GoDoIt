import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, ArrowRight, Check, Download, LoaderCircle, PackagePlus } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { IconPicker } from '../components/IconPicker'
import { api, type EngineChannel } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { IconStrategy, SDKChannel } from '../types'
import { readableError } from '../utils'

const steps = ['Engine', 'Runtime', 'Template', 'Review']

export function NewInstancePage() {
  const navigate = useNavigate()
  const snapshot = useAppStore((state) => state.snapshot)
  const load = useAppStore((state) => state.load)
  const notify = useAppStore((state) => state.notify)
  const [step, setStep] = useState(0)
  const [channels, setChannels] = useState<EngineChannel[]>([])
  const [sdkChannels, setSDKChannels] = useState<SDKChannel[]>([])
  const [loadingVersions, setLoadingVersions] = useState(true)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [source, setSource] = useState('')
  const [version, setVersion] = useState('')
  const [edition, setEdition] = useState<'standard' | 'dotnet'>('standard')
  const [sdkStrategy, setSDKStrategy] = useState<'managed' | 'system'>('managed')
  const [sdkVersion, setSDKVersion] = useState('')
  const [template, setTemplate] = useState(false)
  const [setCurrent, setSetCurrent] = useState(!snapshot?.current)
  const [icon, setIcon] = useState<IconStrategy>('default')
  const [customIcon, setCustomIcon] = useState('')
  const [iconBackground, setIconBackground] = useState('')

  useEffect(() => {
    let active = true
    setLoadingVersions(true)
    runOperation<EngineChannel[]>(api.ListAvailableVersions(source)).then((items) => {
      if (!active) return
      setChannels(items)
      const first = items.flatMap((channel) => channel.versions)[0]
      if (first) setVersion((current) => current || first.version)
    }).catch((reason) => active && setError(readableError(reason))).finally(() => active && setLoadingVersions(false))
    return () => { active = false }
  }, [source])

  useEffect(() => {
    if (edition !== 'dotnet' || sdkStrategy !== 'managed' || sdkChannels.length) return
    runOperation<SDKChannel[]>(api.ListAvailableSDKs()).then((items) => {
      setSDKChannels(items)
      setSDKVersion((current) => current || items[0]?.versions[0] || '')
    }).catch((reason) => setError(readableError(reason)))
  }, [edition, sdkChannels.length, sdkStrategy])

  const available = useMemo(() => channels.flatMap((channel) => channel.versions).filter((item) => item.editions.includes(edition)), [channels, edition])
  useEffect(() => { if (available.length && !available.some((item) => item.version === version)) setVersion(available[0].version) }, [available, version])
  const canContinue = step === 0 ? Boolean(name && version) : step === 1 ? edition === 'standard' || sdkStrategy === 'system' || Boolean(sdkVersion) : true

  const pickCustom = async () => {
    const path = await api.PickIconFile()
    if (path) { setCustomIcon(path); setIcon('custom') }
  }

  const install = async () => {
    setInstalling(true)
    setError('')
    try {
      await runOperation(api.InstallEntry({ name, version, edition, source: source || undefined, sdk_strategy: edition === 'dotnet' ? sdkStrategy : undefined, sdk_version: edition === 'dotnet' && sdkStrategy === 'managed' ? sdkVersion : undefined, template, set_current: setCurrent }))
      let iconError = ''
      if (icon !== 'default' || iconBackground) {
        try {
          await runOperation(api.SetInstanceIcon(name, { icon, source_path: icon === 'custom' ? customIcon : undefined, background: iconBackground || undefined }))
        } catch (reason) {
          iconError = readableError(reason)
        }
      }
      await load()
      notify(iconError ? `${name} 已安装，图标未更新：${iconError}` : `${name} 已安装`)
      navigate(`/instances/${encodeURIComponent(name)}`)
    } catch (reason) {
      setError(readableError(reason))
    } finally {
      setInstalling(false)
    }
  }

  return (
    <div className="page wizard-page">
      <header className="page-header"><div><p className="eyebrow">新建条目</p><h1>{steps[step]}</h1></div><button className="icon-button bordered" type="button" aria-label="关闭向导" onClick={() => navigate('/instances')}>×</button></header>
      <ol className="wizard-steps">{steps.map((label, index) => <li key={label} className={index === step ? 'active' : index < step ? 'complete' : ''}><span>{index < step ? <Check /> : index + 1}</span><strong>{label}</strong></li>)}</ol>
      {error && <div className="inline-error" role="alert">{error}</div>}
      <div className="wizard-content">
        {step === 0 && <section className="wizard-step"><h2>选择引擎</h2><div className="form-grid"><label><span>条目名</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="例如 studio-csharp" /></label><label><span>下载来源</span><select value={source} onChange={(event) => setSource(event.target.value)}><option value="">自动 fallback</option>{snapshot?.assets.sources.filter((item) => !item.disabled).map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}</select></label></div><div className="segmented" aria-label="edition"><button className={edition === 'standard' ? 'active' : ''} type="button" onClick={() => setEdition('standard')}>Standard</button><button className={edition === 'dotnet' ? 'active' : ''} type="button" onClick={() => setEdition('dotnet')}>.NET / C#</button></div>{loadingVersions ? <div className="loading-line"><LoaderCircle className="spin" />正在枚举版本</div> : <div className="version-grid">{available.map((item) => <button type="button" key={item.version} className={version === item.version ? 'selected' : ''} onClick={() => setVersion(item.version)}><strong>{item.version}</strong><small>{item.sources.join(' · ')}</small></button>)}</div>}</section>}
        {step === 1 && <section className="wizard-step"><h2>配置运行时</h2><div className="choice-list"><label className={edition === 'standard' ? 'selected' : ''}><input type="radio" checked={edition === 'standard'} onChange={() => setEdition('standard')} /><span><strong>Standard</strong><small>无需 .NET SDK</small></span></label><label className={edition === 'dotnet' ? 'selected' : ''}><input type="radio" checked={edition === 'dotnet'} onChange={() => setEdition('dotnet')} /><span><strong>.NET / C#</strong><small>使用托管或系统 SDK</small></span></label></div>{edition === 'dotnet' && <><div className="segmented"><button type="button" className={sdkStrategy === 'managed' ? 'active' : ''} onClick={() => setSDKStrategy('managed')}>Managed</button><button type="button" className={sdkStrategy === 'system' ? 'active' : ''} onClick={() => setSDKStrategy('system')}>System</button></div>{sdkStrategy === 'managed' && <label className="field-block"><span>SDK patch</span><select value={sdkVersion} onChange={(event) => setSDKVersion(event.target.value)}>{sdkChannels.flatMap((channel) => channel.versions.map((item) => <option key={item} value={item}>{item} · {channel.release_type.toUpperCase()}</option>))}</select></label>}</>}</section>}
        {step === 2 && <section className="wizard-step"><h2>导出资源</h2><label className={`template-choice ${template ? 'selected' : ''}`}><input type="checkbox" checked={template} onChange={(event) => setTemplate(event.target.checked)} /><span className="round-icon"><Download /></span><span><strong>安装匹配的导出模板</strong><small>{version}-{edition}</small></span></label><h3>条目图标</h3><IconPicker value={icon} customPath={customIcon} background={iconBackground} onChange={setIcon} onBackgroundChange={setIconBackground} onPickCustom={() => void pickCustom()} /></section>}
        {step === 3 && <section className="wizard-step review-step"><h2>确认安装</h2><dl><div><dt>条目</dt><dd>{name}</dd></div><div><dt>引擎</dt><dd>Godot {version} · {edition}</dd></div><div><dt>下载来源</dt><dd>{source || '自动 fallback'}</dd></div><div><dt>SDK</dt><dd>{edition === 'dotnet' ? `${sdkStrategy}${sdkVersion ? ` · ${sdkVersion}` : ''}` : '无'}</dd></div><div><dt>导出模板</dt><dd>{template ? `${version}-${edition}` : '不安装'}</dd></div><div><dt>图标背景</dt><dd>{iconBackground || '透明'}</dd></div><div><dt>当前条目</dt><dd><label className="toggle"><input type="checkbox" checked={setCurrent} onChange={(event) => setSetCurrent(event.target.checked)} /><span />{setCurrent ? '安装后设为当前' : '保持当前条目'}</label></dd></div></dl></section>}
      </div>
      <footer className="wizard-footer"><button className="button secondary" type="button" disabled={step === 0 || installing} onClick={() => setStep((current) => current - 1)}><ArrowLeft />上一步</button>{step < steps.length - 1 ? <button className="button primary" type="button" disabled={!canContinue} onClick={() => setStep((current) => current + 1)}>下一步<ArrowRight /></button> : <button className="button primary" type="button" disabled={installing} onClick={() => void install()}>{installing ? <LoaderCircle className="spin" /> : <PackagePlus />}{installing ? '安装中' : 'Install'}</button>}</footer>
    </div>
  )
}
