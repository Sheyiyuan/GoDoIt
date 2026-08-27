import type {
  AppSnapshot,
  AssetSnapshot,
  DoctorReport,
  EnvironmentDetails,
  EngineCandidateResult,
  EngineChannel,
  InstallEntryRequest,
  InstallSuggestionRequest,
  InstanceDetails,
  OperationStart,
  ProjectSuggestion,
  SDKCandidateResult,
  SDKChannel,
  SetInstanceIconRequest,
  SourceInfo,
  SessionInfo,
  GUISettings,
} from '../types'

type Backend = Record<string, (...args: unknown[]) => Promise<unknown>>

const now = '2026-08-25T08:00:00Z'
let demoSnapshot: AppSnapshot = {
  root: '/home/demo/.gdit',
  instances: [
    { id: '3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8', name: 'studio-csharp', engine: '4.5.2-dotnet', edition: 'dotnet', sdk_strategy: 'managed', sdk: '8.0.410', current: true, template: '4.5.2-dotnet', template_missing: false, icon: 'default', resolved_icon: 'csharp', icon_missing: false, icon_background: '' },
    { id: '7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d', name: 'stable-4.4', engine: '4.4.1-standard', edition: 'standard', sdk_strategy: '', sdk: '', current: false, template: '', template_missing: false, icon: 'godot', resolved_icon: 'godot', icon_missing: false, icon_background: '' },
    { id: '8e5c9f3b-2a7d-4c6e-a1b8-3d9f7a5c2e4b', name: 'legacy-mono', engine: '3.6.2-dotnet', edition: 'dotnet', sdk_strategy: 'mono', sdk: '', current: false, template: '', template_missing: false, icon: 'csharp', resolved_icon: 'csharp', icon_missing: false, icon_background: '#f4f0ff' },
    { id: '9f6d1a4c-3b8e-4d7f-b2c9-4e1a8b6d3f5c', name: 'preview-lab', engine: '4.8-dev3-standard', edition: 'standard', sdk_strategy: '', sdk: '', current: false, template: '4.8-dev3-standard', template_missing: true, icon: 'mascot', resolved_icon: 'mascot', icon_missing: false, icon_background: '' },
  ],
  assets: {
    engines: [
      { id: '4.5.2-dotnet', version: '4.5.2', edition: 'dotnet', target: { os: 'linux', arch: 'amd64' }, source: 'godothub', checksum_algorithm: 'sha256', checksum: 'fixture', launcher: 'Godot', installed_at: now },
      { id: '4.4.1-standard', version: '4.4.1', edition: 'standard', target: { os: 'linux', arch: 'amd64' }, source: 'github', checksum_algorithm: 'sha512', checksum: 'fixture', launcher: 'Godot', installed_at: now },
      { id: '3.6.2-dotnet', version: '3.6.2', edition: 'dotnet', target: { os: 'linux', arch: 'amd64' }, source: 'github', checksum_algorithm: 'sha512', checksum: 'fixture', launcher: 'Godot', installed_at: now },
      { id: '4.8-dev3-standard', version: '4.8-dev3', edition: 'standard', target: { os: 'linux', arch: 'amd64' }, source: 'godothub', checksum_algorithm: 'sha256', checksum: 'fixture', launcher: 'Godot', installed_at: now },
    ],
    sdks: [{ version: '8.0.410', kind: 'managed', path: '/home/demo/.gdit/sdks/8.0.410' }],
    sources: [{ name: 'godothub', kind: 'builtin', disabled: false }, { name: 'github', kind: 'builtin', disabled: false }],
    templates: [{ id: '4.5.2-dotnet', version: '4.5.2', edition: 'dotnet', source: 'godothub', checksum_algorithm: 'sha256', checksum: 'fixture', archive_name: 'Godot_v4.5.2-stable_mono_export_templates.tpz', path: '/home/demo/.gdit/templates/4.5.2-dotnet/payload', size: 1288490188, installed_at: now, references: ['studio-csharp'] }],
    orphans: [{ kind: 'engine', id: '4.3.0-standard', size: 148478361, path: '/home/demo/.gdit/engines/4.3.0-standard' }],
  },
  doctor: {
    root: '/home/demo/.gdit', ok_count: 9, warn_count: 1, error_count: 0,
    items: [
      { code: 'platform', status: 'ok', message: '平台 linux/amd64 受支持' },
      { code: 'current', status: 'ok', message: 'current 指向合法条目' },
      { code: 'instances', status: 'ok', message: '全部 4 个条目引用完整' },
      { code: 'templates', status: 'warn', message: 'preview-lab 的导出模板尚未安装', suggest: '在条目详情中下载模板' },
      { code: 'sources', status: 'ok', message: '2 个来源配置有效', details: ['GodotHub 配置有效', 'GitHub 配置有效'] },
    ],
  },
  gui: { titlebar_style: 'auto' },
  build: { version: '0.2.0-dev', go_version: 'go1.25.13', commit: 'ed9f05e9bdd3' },
}
demoSnapshot.current = demoSnapshot.instances[0]

const demoVersions: EngineChannel[] = [
  { name: '4.x', versions: [{ version: '4.7.2', editions: ['standard', 'dotnet'], sources: ['godothub', 'github'] }, { version: '4.6.2', editions: ['standard', 'dotnet'], sources: ['godothub', 'github'] }, { version: '4.5.2', editions: ['standard', 'dotnet'], sources: ['godothub', 'github'] }] },
  { name: '3.x', versions: [{ version: '3.6.2', editions: ['standard', 'dotnet'], sources: ['github'] }] },
  { name: 'unstable', versions: [{ version: '4.8-dev3', editions: ['standard', 'dotnet'], sources: ['godothub'] }] },
]

const demoGlobalEnvironment: Record<string, string> = {
  GODOT_EDITOR_SCALE: '1.1',
  GITHUB_TOKEN: 'demo-token-value',
}
const demoInstanceEnvironment: Record<string, Record<string, string>> = {
  'studio-csharp': { GODOT4_EDITOR_SETTINGS_PATH: '/home/demo/.config/godot-studio', INSTANCE_TOKEN: 'studio-secret' },
  'preview-lab': { GODOT_LOG_LEVEL: 'debug' },
}
let demoSessions: SessionInfo[] = []
let demoPID = 4600

function demoEnvironment(name: string): EnvironmentDetails {
  const configured: EnvironmentDetails['configured']['vars'] = [
    ...Object.entries(demoGlobalEnvironment).map(([key, value]) => ({ key, value, scope: 'global' as const, editable: true, sensitive: sensitiveEnvironmentKey(key) })),
    { key: 'DISPLAY_BACKEND', value: 'wayland', scope: 'platform' as const, editable: false, sensitive: false },
  ]
  if (name) {
    configured.push(...Object.entries(demoInstanceEnvironment[name] || {}).map(([key, value]) => ({ key, value, scope: 'instance' as const, editable: true, sensitive: sensitiveEnvironmentKey(key) })))
  }
  const effective = new Map<string, { key: string; value: string; origin: string; sensitive: boolean }>()
  configured.forEach((item) => effective.set(item.key, { key: item.key, value: item.value, origin: item.scope, sensitive: item.sensitive }))
  if (name === 'studio-csharp') {
    effective.set('DOTNET_ROOT', { key: 'DOTNET_ROOT', value: '/home/demo/.gdit/sdks/8.0.410', origin: 'derived', sensitive: false })
  }
  return { configured: { vars: configured }, effective: { vars: [...effective.values()], args: [] } }
}

function sensitiveEnvironmentKey(key: string) {
  return /(TOKEN|SECRET|PASSWORD|CREDENTIAL|PRIVATE_KEY)/i.test(key)
}

function emitDemoSession(session: SessionInfo) {
  window.dispatchEvent(new CustomEvent('gdit:session', { detail: clone(session) }))
}

function demoVersionsForSource(source: string): EngineChannel[] {
  if (!source) return clone(demoVersions)
  return demoVersions
    .map((channel) => ({ ...channel, versions: channel.versions.filter((item) => item.sources.includes(source)) }))
    .filter((channel) => channel.versions.length > 0)
}

function backend(): Backend | undefined {
  return window.go?.bridge?.App
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const app = backend()
  if (!app?.[method]) throw new Error(`Bridge method ${method} is unavailable`)
  return app[method](...args) as Promise<T>
}

function clone<T>(value: T): T {
  return structuredClone(value)
}

function demoOperation<T>(operation: string, result: () => T): Promise<OperationStart> {
  const operationID = crypto.randomUUID().replaceAll('-', '')
  const emit = (detail: unknown) => window.dispatchEvent(new CustomEvent('gdit:progress', { detail }))
  setTimeout(() => emit({ operation_id: operationID, timestamp: new Date().toISOString(), status: 'running', operation, progress: { stage: 'resolve', message: '正在解析可用资源' } }), 80)
  setTimeout(() => emit({ operation_id: operationID, timestamp: new Date().toISOString(), status: 'complete', operation, result: result() }), 520)
  return Promise.resolve({ operation_id: operationID })
}

export const api = {
  Bootstrap: () => backend() ? call<AppSnapshot>('Bootstrap') : Promise.resolve(clone(demoSnapshot)),
  GetRoot: () => backend() ? call<string>('GetRoot') : Promise.resolve(demoSnapshot.root),
  GetGUISettings: () => backend() ? call<GUISettings>('GetGUISettings') : Promise.resolve(clone(demoSnapshot.gui)),
  SetGUISettings: (settings: GUISettings) => backend() ? call<void>('SetGUISettings', settings) : Promise.resolve().then(() => { demoSnapshot.gui = clone(settings) }),
  ListAssets: () => backend() ? call<AssetSnapshot>('ListAssets') : Promise.resolve(clone(demoSnapshot.assets)),
  GetInstanceDetails: (name: string) => {
    if (backend()) return call<InstanceDetails>('GetInstanceDetails', name)
    const environment = demoEnvironment(name)
    return Promise.resolve({ instance: clone(demoSnapshot.instances.find((item) => item.name === name)!), env: { vars: environment.effective.vars.map(({ key, value, origin }) => ({ key, value, origin })), args: environment.effective.args }, configured_env: environment.configured, templates: clone(demoSnapshot.assets.templates) })
  },
  ListAvailableVersions: (source = '') => backend() ? call<EngineCandidateResult>('ListAvailableVersions', source) : Promise.resolve({ channels: demoVersionsForSource(source), warnings: [] }),
  ListAvailableSDKs: () => backend() ? call<SDKCandidateResult>('ListAvailableSDKs') : Promise.resolve({ channels: [{ major_minor: '10.0', phase: 'active', release_type: 'lts', versions: ['10.0.103'] }, { major_minor: '8.0', phase: 'maintenance', release_type: 'lts', versions: ['8.0.410', '8.0.408'] }], warnings: [] }),
  GetDoctor: (network: boolean) => backend() ? call<OperationStart>('GetDoctor', network) : demoOperation('doctor', () => clone(demoSnapshot.doctor)),
  Suggest: (projectDir: string) => backend() ? call<OperationStart>('Suggest', projectDir) : demoOperation('suggest', () => ({ project_dir: projectDir, engine_series: '4.5', edition: 'dotnet', sdk_strategy: 'managed', sdk_version: '8.0.410', sdk_channel: '8.0', installable: true, evidence: [{ kind: 'project-feature', path: `${projectDir}/project.godot`, value: '4.5, C#' }, { kind: 'global-json', path: `${projectDir}/global.json`, value: '8.0.410' }, { kind: 'target-framework', path: `${projectDir}/Game.csproj`, value: 'net8.0' }], diagnostics: [{ level: 'warning', code: 'missing-csproj-peer', path: `${projectDir}/project.godot`, message: '建议确认所有 C# 项目文件位于同一目录' }] } satisfies ProjectSuggestion)),
  InstallEntry: (request: InstallEntryRequest) => backend() ? call<OperationStart>('InstallEntry', request) : demoOperation('install-entry', () => {
    const item = { id: crypto.randomUUID(), name: request.name, engine: `${request.version}-${request.edition}`, edition: request.edition as 'standard' | 'dotnet', sdk_strategy: request.sdk_strategy || '', sdk: request.sdk_version || '', current: Boolean(request.set_current), template: request.template ? `${request.version}-${request.edition}` : '', template_missing: false, icon: 'default' as const, resolved_icon: request.edition === 'dotnet' ? 'csharp' as const : 'godot' as const, icon_missing: false, icon_background: '' }
    if (request.set_current) demoSnapshot.instances.forEach((entry) => { entry.current = false })
    demoSnapshot.instances.push(item)
    if (request.set_current) demoSnapshot.current = item
    return { instance: item, installed: [{ kind: 'engine', id: item.engine }] }
  }),
  InstallSuggestion: (request: InstallSuggestionRequest) => backend() ? call<OperationStart>('InstallSuggestion', request) : demoOperation('install-suggestion', () => ({ engine_version: '4.5.2', suggestion: {}, entry: {} })),
  RemoveInstance: (name: string) => backend() ? call<OperationStart>('RemoveInstance', name) : demoOperation('remove-instance', () => { demoSnapshot.instances = demoSnapshot.instances.filter((item) => item.name !== name); return { instance: { name }, orphans: [] } }),
  AutoRemove: () => backend() ? call<OperationStart>('AutoRemove') : demoOperation('autoremove', () => { const removed = clone(demoSnapshot.assets.orphans); demoSnapshot.assets.orphans = []; return { removed } }),
  AttachTemplate: (name: string, source = '') => backend() ? call<OperationStart>('AttachTemplate', name, source) : demoOperation('attach-template', () => ({ instance: { name }, template: { id: demoSnapshot.instances.find((item) => item.name === name)?.engine }, installed: true })),
  DetachTemplate: (name: string) => backend() ? call<OperationStart>('DetachTemplate', name) : demoOperation('detach-template', () => ({ instance: { name } })),
  SetInstanceIcon: (name: string, request: SetInstanceIconRequest) => backend() ? call<OperationStart>('SetInstanceIcon', name, request) : demoOperation('set-instance-icon', () => { const item = demoSnapshot.instances.find((entry) => entry.name === name)!; item.icon = request.icon; item.resolved_icon = request.icon === 'default' ? (item.edition === 'dotnet' ? 'csharp' : 'godot') : request.icon as InstanceDetails['instance']['resolved_icon']; item.icon_background = request.background || ''; return clone(item) }),
  GetEnvironment: (name: string) => backend() ? call<EnvironmentDetails>('GetEnvironment', name) : Promise.resolve(clone(demoEnvironment(name))),
  ListSessions: () => backend() ? call<{ sessions: SessionInfo[] }>('ListSessions') : Promise.resolve({ sessions: clone(demoSessions) }),
  LaunchSession: async (name: string) => {
    if (backend()) return call<SessionInfo>('LaunchSession', name)
    const instance = demoSnapshot.instances.find((item) => item.name === name)
    if (!instance) throw new Error(`条目不存在：${name}`)
    const session: SessionInfo = { session_id: crypto.randomUUID(), instance_id: instance.id, instance_name: name, engine_id: instance.engine, pid: demoPID++, started_at: new Date().toISOString(), status: 'running' }
    demoSessions.push(session)
    emitDemoSession(session)
    return clone(session)
  },
  RequestStopSession: async (id: string) => {
    if (backend()) return call<SessionInfo>('RequestStopSession', id)
    const session = demoSessions.find((item) => item.session_id === id)
    if (!session) throw new Error(`会话不存在：${id}`)
    session.status = 'stopping'
    emitDemoSession(session)
    return clone(session)
  },
  ForceStopSession: async (id: string) => {
    if (backend()) return call<SessionInfo>('ForceStopSession', id)
    const session = demoSessions.find((item) => item.session_id === id)
    if (!session) throw new Error(`会话不存在：${id}`)
    const exited: SessionInfo = { ...session, status: 'exited' }
    demoSessions = demoSessions.filter((item) => item.session_id !== id)
    emitDemoSession(exited)
    return clone(exited)
  },
  SetDefault: async (name: string) => { if (backend()) return call<void>('SetDefault', name); demoSnapshot.instances.forEach((item) => { item.current = item.name === name }); demoSnapshot.current = demoSnapshot.instances.find((item) => item.name === name) },
  SetEnvVar: (scope: string, key: string, value: string) => backend() ? call<void>('SetEnvVar', scope, key, value) : Promise.resolve().then(() => { const target = scope ? (demoInstanceEnvironment[scope] ||= {}) : demoGlobalEnvironment; target[key] = value }),
  UnsetEnvVar: (scope: string, key: string) => backend() ? call<void>('UnsetEnvVar', scope, key) : Promise.resolve().then(() => { const target = scope ? demoInstanceEnvironment[scope] : demoGlobalEnvironment; if (target) delete target[key] }),
  ListSources: () => backend() ? call<SourceInfo[]>('ListSources') : Promise.resolve(clone(demoSnapshot.assets.sources)),
  SetSourceDisabled: async (name: string, disabled: boolean) => { if (backend()) return call<void>('SetSourceDisabled', name, disabled); const source = demoSnapshot.assets.sources.find((item) => item.name === name); if (source) source.disabled = disabled },
  SetDefaultSource: async (name: string) => { if (backend()) return call<void>('SetDefaultSource', name); demoSnapshot.assets.sources.sort((item) => item.name === name ? -1 : 1) },
  Launch: (name: string) => backend() ? call<void>('Launch', name) : Promise.resolve(),
  PickProjectDirectory: () => backend() ? call<string>('PickProjectDirectory') : Promise.resolve('/home/demo/projects/space-game'),
  PickIconFile: () => backend() ? call<string>('PickIconFile') : Promise.resolve('/home/demo/Pictures/icon.png'),
  Cancel: (id: string) => backend() ? call<boolean>('Cancel', id) : Promise.resolve(true),
}

export type { DoctorReport, EngineChannel }
