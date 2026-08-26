import { useEffect, useState } from 'react'
import { Plus, Waypoints } from 'lucide-react'
import { EnvironmentEditor } from '../components/EnvironmentEditor'
import { useAppStore } from '../store/app'
import type { TitlebarStyle } from '../types'
import { readableError } from '../utils'

export function SettingsPage() {
  const snapshot = useAppStore((state) => state.snapshot)
  const notify = useAppStore((state) => state.notify)
  const updateGUISettings = useAppStore((state) => state.updateGUISettings)
  const [error, setError] = useState('')
  const [titlebarStyle, setTitlebarStyle] = useState<TitlebarStyle>('auto')

  useEffect(() => {
    if (snapshot?.gui) setTitlebarStyle(snapshot.gui.titlebar_style)
  }, [snapshot?.gui])

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
      <div className="page-body">
        {error && <div className="inline-error">{error}</div>}
        <section className="settings-band"><div className="section-heading"><div><h2>数据位置</h2><p>当前 gdit 根目录</p></div></div><code className="root-path">{snapshot?.root}</code></section>
        <section className="settings-band"><div className="section-heading"><div><h2>窗口顶栏</h2><p>无边框窗口控制位置</p></div></div><div className="titlebar-style-setting"><div className="segmented" role="group" aria-label="窗口顶栏风格"><button className={titlebarStyle === 'auto' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('auto')}>跟随系统</button><button className={titlebarStyle === 'mac' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('mac')}>左上角红绿灯</button><button className={titlebarStyle === 'windows' ? 'active' : ''} type="button" onClick={() => void updateTitlebarStyle('windows')}>右上角窗口按钮</button></div></div></section>
        <section className="settings-band environment-settings"><div className="section-heading"><div><h2>全局环境</h2><p>[environment] 与当前平台默认值</p></div></div><EnvironmentEditor name="" editableScope="global" /></section>
        <section className="settings-band"><div className="section-heading"><div><h2>下载来源</h2><p>顺序与启禁用</p></div></div><div className="compact-source-list">{snapshot?.assets.sources.map((source, index) => <div key={source.name}><Waypoints /><span><strong>{source.name}</strong><small>{source.kind}</small></span><em>{source.disabled ? '禁用' : index === 0 ? '默认' : `优先级 ${index + 1}`}</em></div>)}</div><p className="config-note"><Plus />自定义来源 URL 继续在 config.toml 中维护</p></section>
      </div>
    </div>
  )
}
