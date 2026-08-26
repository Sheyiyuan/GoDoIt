export type IconStrategy = 'default' | 'godot' | 'csharp' | 'mascot' | 'custom'
export type TitlebarStyle = 'auto' | 'mac' | 'windows'

export interface GUISettings {
  titlebar_style: TitlebarStyle
}

export interface InstanceInfo {
  id: string
  name: string
  engine: string
  edition: 'standard' | 'dotnet'
  sdk_strategy: string
  sdk: string
  current: boolean
  template: string
  template_missing: boolean
  icon: IconStrategy
  resolved_icon: Exclude<IconStrategy, 'default'>
  icon_missing: boolean
  icon_background: string
}

export interface InstalledVersion {
  id: string
  version: string
  edition: string
  target: { os: string; arch: string }
  source: string
  checksum_algorithm: string
  checksum: string
  launcher: string
  installed_at: string
}

export interface SDKInfo {
  version: string
  kind: string
  path: string
}

export interface SourceInfo {
  name: string
  kind: string
  disabled: boolean
}

export interface TemplateInfo {
  id: string
  version: string
  edition: string
  source: string
  checksum_algorithm: string
  checksum: string
  archive_name: string
  path: string
  size: number
  installed_at: string
  references: string[]
}

export interface OrphanAsset {
  kind: string
  id: string
  size: number
  path: string
}

export interface AssetSnapshot {
  engines: InstalledVersion[]
  sdks: SDKInfo[]
  sources: SourceInfo[]
  templates: TemplateInfo[]
  orphans: OrphanAsset[]
}

export interface EnvVar {
  key: string
  value: string
  origin: string
}

export interface EnvView {
  vars: EnvVar[]
  args: string[]
}

export type EnvScope = 'global' | 'platform' | 'instance'

export interface ConfiguredEnvVar {
  key: string
  value: string
  scope: EnvScope
  editable: boolean
  sensitive: boolean
}

export interface ConfiguredEnvView {
  vars: ConfiguredEnvVar[]
}

export interface InstanceDetails {
  instance: InstanceInfo
  env: EnvView
  configured_env: ConfiguredEnvView
  env_error?: string
  templates: TemplateInfo[]
}

export interface EnvironmentDetails {
  configured: ConfiguredEnvView
  effective: EnvView
  effective_error?: string
}

export interface SessionInfo {
  session_id: string
  instance_id: string
  instance_name: string
  engine_id: string
  pid: number
  started_at: string
  status: 'running' | 'stopping' | 'exited' | 'lost'
}

export type CheckStatus = 'ok' | 'warn' | 'error'

export interface CheckResult {
  code: string
  status: CheckStatus
  message: string
  suggest?: string
  details?: string[]
}

export interface DoctorReport {
  root: string
  items: CheckResult[]
  ok_count: number
  warn_count: number
  error_count: number
}

export interface BuildInfo {
  version: string
  go_version: string
  commit?: string
}

export interface AppSnapshot {
  root: string
  instances: InstanceInfo[]
  current?: InstanceInfo
  assets: AssetSnapshot
  doctor: DoctorReport
  gui: GUISettings
  build: BuildInfo
  issues?: string[]
}

export interface AvailableVersion {
  version: string
  editions: string[]
  sources: string[]
}

export interface EngineChannel {
  name: string
  versions: AvailableVersion[]
}

export interface SDKChannel {
  major_minor: string
  phase: string
  release_type: string
  versions: string[]
}

export interface SuggestDiagnostic {
  level: 'warning' | 'error'
  code: string
  path?: string
  message: string
}

export interface SuggestEvidence {
  kind: string
  path: string
  value: string
}

export interface ProjectSuggestion {
  project_dir: string
  engine_series: string
  edition: 'standard' | 'dotnet'
  sdk_strategy: string
  sdk_version: string
  sdk_channel: string
  evidence: SuggestEvidence[]
  diagnostics: SuggestDiagnostic[]
  installable: boolean
}

export interface InstallEntryRequest {
  name: string
  version: string
  edition: string
  source?: string
  sdk_strategy?: string
  sdk_version?: string
  set_current?: boolean
  template?: boolean
}

export interface InstallEntryResult {
  instance: InstanceInfo
  installed: Array<{ kind: string; id: string }>
  state_rebuild_required?: boolean
}

export interface InstallSuggestionRequest {
  project_dir: string
  name: string
  sdk_strategy?: string
  sdk_version?: string
  set_current?: boolean
  include_template?: boolean
}

export interface SetInstanceIconRequest {
  icon: IconStrategy
  source_path?: string
  background?: string
}

export interface ProgressEvent {
  stage: string
  version?: string
  source?: string
  filename?: string
  bytes_downloaded?: number
  total_bytes?: number
  message?: string
}

export interface ProgressEnvelope<T = unknown> {
  operation_id: string
  timestamp: string
  status: 'running' | 'complete' | 'failed' | 'canceled'
  operation: string
  progress?: ProgressEvent
  result?: T
  error?: string
}

export interface OperationItem {
  key: string
  version?: string
  source?: string
  filename?: string
  stage: string
  bytes_downloaded?: number
  total_bytes?: number
}

export interface OperationRecord {
  id: string
  operation: string
  status: ProgressEnvelope['status']
  started_at: string
  finished_at?: string
  summary?: string
  error?: string
  result_summary: string[]
  items: OperationItem[]
}

export interface OperationStart {
  operation_id: string
}
