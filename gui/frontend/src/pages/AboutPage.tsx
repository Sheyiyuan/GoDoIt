import { ExternalLink, Github, Terminal } from 'lucide-react'
import { BrandMark } from '../components/BrandMark'
import { useAppStore } from '../store/app'

export function AboutPage() {
  const snapshot = useAppStore((state) => state.snapshot)
  return <div className="page about-page"><section className="about-hero"><BrandMark large /><div><h1>GoDoIt</h1><p>够独特</p></div></section><dl className="about-details"><div><dt>版本</dt><dd>{snapshot?.build.version}</dd></div><div><dt>Commit</dt><dd>{snapshot?.build.commit || 'development'}</dd></div><div><dt>Go</dt><dd>{snapshot?.build.go_version}</dd></div><div><dt>CLI</dt><dd><Terminal />gdit</dd></div></dl><div className="about-links"><a className="button secondary" href="https://github.com/Sheyiyuan/GoDoIt" target="_blank" rel="noreferrer"><Github />GitHub<ExternalLink /></a></div><img className="about-mascot" src="/mascot.png" alt="GoDoIt 吉祥物" /></div>
}
