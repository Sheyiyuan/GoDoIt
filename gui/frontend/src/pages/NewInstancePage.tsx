import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, ArrowRight, Check, Download, LoaderCircle, PackagePlus, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { IconPicker } from '../components/IconPicker'
import { api } from '../lib/api'
import { runOperation } from '../lib/operations'
import { useAppStore } from '../store/app'
import type { CandidateWarning, IconStrategy, InstallEntryResult } from '../types'
import { readableError } from '../utils'

const steps = ['配置', '确认']

export function NewInstancePage() {
  const navigate = useNavigate()
  const snapshot = useAppStore((state) => state.snapshot)
  const load = useAppStore((state) => state.load)
  const notify = useAppStore((state) => state.notify)
  const applyInstalledInstance = useAppStore((state) => state.applyInstalledInstance)
  const [source, setSource] = useState('')
  const cachedEngine = useAppStore((state) => state.engineCandidates[source])
  const cachedSDK = useAppStore((state) => state.sdkCandidates)
  const refreshEngineCandidates = useAppStore((state) => state.refreshEngineCandidates)
  const refreshSDKCandidates = useAppStore((state) => state.refreshSDKCandidates)
  const [step, setStep] = useState(0)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [engineChannel, setEngineChannel] = useState('')
  const [version, setVersion] = useState('')
  const [edition, setEdition] = useState<'standard' | 'dotnet'>('standard')
  const [sdkStrategy, setSDKStrategy] = useState<'managed' | 'system'>('managed')
  const [sdkChannel, setSDKChannel] = useState('')
  const [sdkVersion, setSDKVersion] = useState('')
  const [template, setTemplate] = useState(false)
  const [setCurrent, setSetCurrent] = useState(!snapshot?.current)
  const [icon, setIcon] = useState<IconStrategy>('default')
  const [customIcon, setCustomIcon] = useState('')
  const [iconBackground, setIconBackground] = useState('')

  useEffect(() => {
    if (!cachedEngine || cachedEngine.status === 'idle') void refreshEngineCandidates(source)
  }, [cachedEngine, refreshEngineCandidates, source])

  useEffect(() => {
    if (edition !== 'dotnet' || sdkStrategy !== 'managed') return
    if (cachedSDK.status === 'loading') return
    if (cachedSDK.status === 'ready' || cachedSDK.status === 'error') return
    void refreshSDKCandidates()
  }, [cachedSDK.status, edition, refreshSDKCandidates, sdkStrategy])

  const engineChannels = useMemo(() => (cachedEngine?.items || []).filter((channel) => channel.versions.some((item) => item.editions.includes(edition))), [cachedEngine?.items, edition])
  const selectedEngineChannel = engineChannels.find((channel) => channel.name === engineChannel) || engineChannels[0]
  const available = useMemo(() => (selectedEngineChannel?.versions || []).filter((item) => item.editions.includes(edition)), [edition, selectedEngineChannel])
  const selectedSDKChannel = cachedSDK.items.find((channel) => channel.major_minor === sdkChannel) || cachedSDK.items[0]
  const usesMono = edition === 'dotnet' && selectedEngineChannel?.name === '3.x'

  useEffect(() => {
    const nextChannel = engineChannels.find((channel) => channel.name === engineChannel) || engineChannels[0]
    if (!nextChannel) {
      setEngineChannel('')
      setVersion('')
      return
    }
    if (engineChannel !== nextChannel.name) setEngineChannel(nextChannel.name)
    const versions = nextChannel.versions.filter((item) => item.editions.includes(edition))
    if (!versions.some((item) => item.version === version)) setVersion(versions[0]?.version || '')
  }, [edition, engineChannel, engineChannels, version])

  useEffect(() => {
    const nextChannel = cachedSDK.items.find((channel) => channel.major_minor === sdkChannel) || cachedSDK.items[0]
    if (!nextChannel) {
      setSDKChannel('')
      setSDKVersion('')
      return
    }
    if (sdkChannel !== nextChannel.major_minor) setSDKChannel(nextChannel.major_minor)
    if (sdkVersion && !nextChannel.versions.includes(sdkVersion)) setSDKVersion('')
  }, [cachedSDK.items, sdkChannel, sdkVersion])

  const canContinue = step === 0 ? Boolean(name && version) : true

  const pickCustom = async () => {
    const path = await api.PickIconFile()
    if (path) { setCustomIcon(path); setIcon('custom') }
  }

  const install = async () => {
    setInstalling(true)
    setError('')
    try {
      const result = await runOperation<InstallEntryResult>(api.InstallEntry({ name, version, edition, source: source || undefined, sdk_strategy: edition === 'dotnet' && !usesMono ? sdkStrategy : undefined, sdk_version: edition === 'dotnet' && !usesMono && sdkStrategy === 'managed' ? sdkVersion || undefined : undefined, template, set_current: setCurrent }))
      applyInstalledInstance(result.instance)
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
        {step === 0 && <section className="wizard-step wizard-config">
          <div className="wizard-section">
            <h2>条目信息</h2>
            <div className="form-grid"><label><span>条目名</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="例如 studio-csharp" /></label><label><span>下载来源</span><select value={source} onChange={(event) => setSource(event.target.value)}><option value="">自动 fallback</option>{snapshot?.assets.sources.filter((item) => !item.disabled).map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}</select></label></div>
          </div>
          <div className="wizard-section">
            <div className="wizard-heading-row"><h2>引擎</h2><button className="icon-button" type="button" aria-label="刷新引擎候选" title="刷新引擎候选" onClick={() => void refreshEngineCandidates(source)}><RefreshCw className={cachedEngine?.status === 'loading' ? 'spin' : ''} /></button></div>
            <div className="segmented" aria-label="edition"><button className={edition === 'standard' ? 'active' : ''} type="button" onClick={() => setEdition('standard')}>Standard</button><button className={edition === 'dotnet' ? 'active' : ''} type="button" onClick={() => setEdition('dotnet')}>.NET / C#</button></div>
            {cachedEngine?.status === 'loading' && !engineChannels.length ? <div className="loading-line"><LoaderCircle className="spin" />正在枚举版本</div> : cachedEngine?.status === 'error' && !engineChannels.length ? <div className="loading-line">候选枚举失败：{cachedEngine.error}</div> : !engineChannels.length ? <div className="loading-line">当前来源没有可用于此 edition 的版本</div> : <>
              <div className="channel-picker" role="group" aria-label="引擎系列">{engineChannels.map((channel) => <button type="button" key={channel.name} className={selectedEngineChannel?.name === channel.name ? 'active' : ''} onClick={() => setEngineChannel(channel.name)}>{channel.name}</button>)}</div>
              {cachedEngine?.status === 'error' && <CandidateWarnings warnings={[{ message: cachedEngine.error || '候选刷新失败，继续显示上次结果' }]} />}
              <CandidateWarnings warnings={cachedEngine?.warnings || []} />
              <div className="version-grid">{available.map((item) => <button type="button" key={item.version} className={version === item.version ? 'selected' : ''} onClick={() => setVersion(item.version)}><strong>{item.version}</strong><small>{item.sources.join(' · ')}</small></button>)}</div>
            </>}
          </div>
          <div className="wizard-section">
            <h2>运行时</h2>
            {edition === 'standard' ? <div className="runtime-choice-note"><strong>无需 .NET SDK</strong><small>Standard</small></div> : usesMono ? <div className="runtime-choice-note"><strong>系统 Mono</strong><small>Godot 3.x</small></div> : <><div className="segmented"><button type="button" className={sdkStrategy === 'managed' ? 'active' : ''} onClick={() => setSDKStrategy('managed')}>Managed</button><button type="button" className={sdkStrategy === 'system' ? 'active' : ''} onClick={() => setSDKStrategy('system')}>System</button></div>{sdkStrategy === 'managed' && <div className="sdk-picker">
              {cachedSDK.status === 'loading' && !cachedSDK.items.length ? <div className="loading-line"><LoaderCircle className="spin" />正在枚举 SDK</div> : cachedSDK.status === 'error' && !cachedSDK.items.length ? <div className="loading-line">SDK 候选枚举失败：{cachedSDK.error}</div> : <>
                <label className="field-block"><span>SDK 通道</span><select aria-label="SDK 通道" value={selectedSDKChannel?.major_minor || ''} onChange={(event) => { setSDKChannel(event.target.value); setSDKVersion('') }}>{cachedSDK.items.map((channel) => <option key={channel.major_minor} value={channel.major_minor}>{channel.major_minor} · {channel.release_type.toUpperCase()} · {channel.phase}</option>)}</select></label>
                <label className="field-block"><span>SDK 版本</span><select value={sdkVersion} onChange={(event) => setSDKVersion(event.target.value)}><option value="">推荐版本（由 core 根据 Godot 版本解析）</option>{selectedSDKChannel?.versions.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
                {cachedSDK.status === 'error' && <CandidateWarnings warnings={[{ message: cachedSDK.error || 'SDK 候选刷新失败，继续显示上次结果' }]} />}
                <CandidateWarnings warnings={cachedSDK.warnings} />
              </>}
            </div>}</>}
          </div>
          <div className="wizard-section">
            <h2>可选资源</h2>
            <label className={`template-choice ${template ? 'selected' : ''}`}><input type="checkbox" checked={template} onChange={(event) => setTemplate(event.target.checked)} /><span className="round-icon"><Download /></span><span><strong>导出模板</strong><small>{version}-{edition}</small></span></label>
            <h3>条目图标</h3><IconPicker value={icon} customPath={customIcon} background={iconBackground} onChange={setIcon} onBackgroundChange={setIconBackground} onPickCustom={() => void pickCustom()} />
          </div>
          <div className="wizard-section wizard-current-row">
            <div><h2>当前条目</h2><p>{setCurrent ? '安装后设为当前' : '保持现有当前条目'}</p></div>
            <label className="toggle"><input type="checkbox" checked={setCurrent} onChange={(event) => setSetCurrent(event.target.checked)} /><span /></label>
          </div>
        </section>}
        {step === 1 && <section className="wizard-step review-step"><h2>确认安装</h2><dl><div><dt>条目</dt><dd>{name}</dd></div><div><dt>引擎</dt><dd>Godot {version} · {edition}</dd></div><div><dt>下载来源</dt><dd>{source || '自动 fallback'}</dd></div><div><dt>.NET SDK</dt><dd>{edition !== 'dotnet' ? '不安装' : usesMono ? '系统 Mono' : sdkStrategy === 'system' ? '使用系统 SDK' : sdkVersion || '推荐版本（安装时解析）'}</dd></div><div><dt>导出模板</dt><dd>{template ? `${version}-${edition}` : '不安装'}</dd></div><div><dt>图标背景</dt><dd>{iconBackground || '透明'}</dd></div><div><dt>当前条目</dt><dd>{setCurrent ? '设为当前' : '保持不变'}</dd></div></dl></section>}
      </div>
      <footer className="wizard-footer"><button className="button secondary" type="button" disabled={step === 0 || installing} onClick={() => setStep((current) => current - 1)}><ArrowLeft />上一步</button>{step < steps.length - 1 ? <button className="button primary" type="button" disabled={!canContinue} onClick={() => setStep((current) => current + 1)}>下一步<ArrowRight /></button> : <button className="button primary" type="button" disabled={installing} onClick={() => void install()}>{installing ? <LoaderCircle className="spin" /> : <PackagePlus />}{installing ? '安装中' : 'Install'}</button>}</footer>
    </div>
  )
}

function CandidateWarnings({ warnings }: { warnings: CandidateWarning[] }) {
  if (!warnings.length) return null
  return <div className="candidate-warnings" role="status">{warnings.map((warning, index) => <p key={`${warning.source || ''}:${warning.message}:${index}`}>{warning.source && <strong>{warning.source}</strong>}{warning.message}</p>)}</div>
}
