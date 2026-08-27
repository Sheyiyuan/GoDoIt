import { useState } from 'react'
import { ExternalLink, Github, Scale, Terminal, X } from 'lucide-react'
import { BrandMark } from '../components/BrandMark'
import { useAppStore } from '../store/app'
import licenseText from '../../../../LICENSE?raw'
import thirdPartyNotices from '../../../../THIRD_PARTY_NOTICES.txt?raw'

export function AboutPage() {
  const snapshot = useAppStore((state) => state.snapshot)
  const [legalView, setLegalView] = useState<'license' | 'third-party' | null>(null)
  return <div className="page about-page"><div className="page-body about-body"><section className="about-hero"><BrandMark large /><div><h1>GoDoIt</h1><p>够独特</p></div></section><dl className="about-details"><div><dt>版本</dt><dd>{snapshot?.build.version}</dd></div><div><dt>Commit</dt><dd>{snapshot?.build.commit || 'development'}</dd></div>{snapshot?.build.build_date && <div><dt>构建时间</dt><dd>{snapshot.build.build_date}</dd></div>}<div><dt>Go</dt><dd>{snapshot?.build.go_version}</dd></div><div><dt>CLI</dt><dd><Terminal />gdit</dd></div></dl><div className="about-links"><a className="button secondary" href="https://github.com/Sheyiyuan/GoDoIt" target="_blank" rel="noreferrer"><Github />GitHub<ExternalLink /></a><button className="button secondary" type="button" onClick={() => setLegalView('license')}><Scale />开源许可</button></div>{legalView && <section className="legal-view" aria-labelledby="legal-title"><header><div><h2 id="legal-title">开源许可</h2><small>GoDoIt 与第三方组件</small></div><button className="icon-button" type="button" aria-label="关闭开源许可" title="关闭" onClick={() => setLegalView(null)}><X /></button></header><div className="legal-tabs" role="tablist" aria-label="法律文本"><button type="button" role="tab" aria-selected={legalView === 'license'} onClick={() => setLegalView('license')}>GoDoIt AGPL-3.0</button><button type="button" role="tab" aria-selected={legalView === 'third-party'} onClick={() => setLegalView('third-party')}>第三方声明</button></div><pre tabIndex={0}>{legalView === 'license' ? licenseText : thirdPartyNotices}</pre></section>}<img className="about-mascot" src="/mascot.png" alt="GoDoIt 吉祥物" /></div></div>
}
