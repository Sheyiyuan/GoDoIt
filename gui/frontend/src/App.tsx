import { useEffect } from 'react'
import { LoaderCircle, RefreshCw } from 'lucide-react'
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { BrandMark } from './components/BrandMark'
import { subscribeProgress } from './lib/operations'
import { subscribeSessions } from './lib/sessions'
import { AboutPage } from './pages/AboutPage'
import { DoctorPage } from './pages/DoctorPage'
import { InstancePage } from './pages/InstancePage'
import { NewInstancePage } from './pages/NewInstancePage'
import { ResourcesPage } from './pages/ResourcesPage'
import { SettingsPage } from './pages/SettingsPage'
import { SuggestPage } from './pages/SuggestPage'
import { ToolsPage } from './pages/ToolsPage'
import { useAppStore } from './store/app'
import './App.css'

export default function App() {
  const snapshot = useAppStore((state) => state.snapshot)
  const loading = useAppStore((state) => state.loading)
  const error = useAppStore((state) => state.error)
  const bootstrapRoot = useAppStore((state) => state.bootstrapRoot)
  const load = useAppStore((state) => state.load)
  const handleProgress = useAppStore((state) => state.handleProgress)
  const prefetchCandidates = useAppStore((state) => state.prefetchCandidates)
  const handleSession = useAppStore((state) => state.handleSession)
  const refreshSessions = useAppStore((state) => state.refreshSessions)

  useEffect(() => {
    const unsubscribe = subscribeProgress(handleProgress)
    void load()
    return unsubscribe
  }, [handleProgress, load])

  useEffect(() => {
    if (snapshot) void prefetchCandidates()
  }, [prefetchCandidates, snapshot])

  useEffect(() => {
    if (!snapshot) return
    const unsubscribe = subscribeSessions(handleSession)
    void refreshSessions()
    const timer = window.setInterval(() => void refreshSessions(), 2500)
    return () => { unsubscribe(); window.clearInterval(timer) }
  }, [handleSession, refreshSessions, snapshot])

  if (loading && !snapshot) return <div className="boot-screen"><BrandMark large /><LoaderCircle className="spin" /><span>正在读取 GoDoIt</span></div>
  if (error && !snapshot) return <div className="boot-screen error"><BrandMark large /><strong>无法载入工作台</strong>{bootstrapRoot && <code>{bootstrapRoot}</code>}<p>{error}</p><button className="button primary" type="button" onClick={() => void load()}><RefreshCw />重试</button></div>

  return (
    <HashRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/instances" replace />} />
          <Route path="/instances" element={<InstancePage />} />
          <Route path="/instances/new" element={<NewInstancePage />} />
          <Route path="/instances/:name" element={<InstancePage />} />
          <Route path="/tools" element={<ToolsPage />} />
          <Route path="/resources/:kind" element={<ResourcesPage />} />
          <Route path="/suggest" element={<SuggestPage />} />
          <Route path="/doctor" element={<DoctorPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/about" element={<AboutPage />} />
          <Route path="*" element={<Navigate to="/instances" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}
