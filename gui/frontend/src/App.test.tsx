import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { api } from './lib/api'
import { useAppStore } from './store/app'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAppStore.setState({
    snapshot: null,
    loading: true,
    error: '',
    bootstrapRoot: '',
    engineCandidates: {},
    sdkCandidates: { status: 'idle', items: [], warnings: [] },
    operations: {},
    dismissedWarnings: {},
    sessions: [],
    sessionsLoading: false,
    sessionError: '',
    sessionRevision: 0,
    terminalSessions: {},
  })
  window.location.hash = ''
})

describe('GoDoIt workbench', () => {
  it('shows the failed bootstrap root and retries into a complete workbench', async () => {
    const snapshot = await api.Bootstrap()
    vi.spyOn(api, 'Bootstrap').mockRejectedValueOnce(new Error('initialize store: permission denied')).mockResolvedValue(snapshot)
    vi.spyOn(api, 'GetRoot').mockResolvedValue('/tmp/gdit-retry')
    useAppStore.setState({ snapshot: null, loading: true, error: '', bootstrapRoot: '' })

    render(<App />)

    expect(await screen.findByText('无法载入工作台')).toBeInTheDocument()
    expect(screen.getByText('/tmp/gdit-retry')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('link', { name: '新建条目' })).toBeInTheDocument()
  })

  it('groups bootstrap and doctor issues while excluding optional integration reminders', async () => {
    const snapshot = structuredClone(await api.Bootstrap())
    snapshot.issues = ['current 指针读取失败']
    snapshot.doctor = {
      root: snapshot.root,
      ok_count: 1,
      warn_count: 2,
      error_count: 0,
      items: [
        { code: 'platform', status: 'ok', message: '平台受支持' },
        { code: 'templates', status: 'warn', message: '导出模板缺失', suggest: '在条目详情中下载模板' },
        { code: 'shim', status: 'warn', message: 'godot shim 尚未创建', suggest: '运行 gdit setup' },
      ],
    }
    vi.spyOn(api, 'Bootstrap').mockResolvedValue(snapshot)
    useAppStore.setState({ snapshot: null, loading: true, error: '', dismissedWarnings: {} })

    render(<App />)

    expect(await screen.findByText('发现 2 项问题')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看详情' }))
    expect(screen.getByText('故障')).toBeInTheDocument()
    expect(screen.getByText('需要注意')).toBeInTheDocument()
    expect(screen.getByText('可选集成')).toBeInTheDocument()
    expect(screen.queryByText('godot shim 尚未创建')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '本次关闭' }))
    expect(await screen.findByText('发现 1 项问题')).toBeInTheDocument()
  })

  it('puts the create action before instances and opens the current instance', async () => {
    render(<App />)
    const create = await screen.findByRole('link', { name: '新建条目' })
    const instanceLinks = await screen.findAllByRole('link', { name: /studio-csharp/ })
    expect(create.compareDocumentPosition(instanceLinks[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    await waitFor(() => expect(screen.getByRole('heading', { name: 'studio-csharp' })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '启动 Godot' })).toBeEnabled()
  })

  it('shows all GUI sessions globally and requires confirmation before force stop', async () => {
    const running = { session_id: 'session-1', instance_id: 'instance-1', instance_name: 'studio-csharp', engine_id: '4.5.2-dotnet', pid: 4200, started_at: '2026-08-26T01:00:00Z', status: 'running' as const }
    vi.spyOn(api, 'ListSessions').mockResolvedValue({ sessions: [running] })
    vi.spyOn(api, 'RequestStopSession').mockResolvedValue({ ...running, status: 'stopping' })
    const forceStop = vi.spyOn(api, 'ForceStopSession').mockResolvedValue({ ...running, status: 'exited' })

    render(<App />)
    const sessionButton = await screen.findByRole('button', { name: '运行中 1' })
    fireEvent.click(sessionButton)
    const panel = screen.getByRole('dialog', { name: '运行会话' })
    expect(within(panel).getByText('4.5.2-dotnet · PID 4200')).toBeInTheDocument()
    fireEvent.click(within(panel).getByRole('button', { name: '关闭' }))
    expect(await within(panel).findByRole('button', { name: '等待退出' })).toBeDisabled()
    const forceButton = await within(panel).findByRole('button', { name: '强制结束' }, { timeout: 3000 })
    fireEvent.click(forceButton)
    expect(forceStop).not.toHaveBeenCalled()
    const confirmation = screen.getByRole('dialog', { name: /强制结束 studio-csharp/ })
    fireEvent.click(within(confirmation).getByRole('button', { name: '强制结束' }))
    await waitFor(() => expect(forceStop).toHaveBeenCalledWith('session-1'))
    expect(await screen.findByRole('button', { name: '运行中 0' })).toBeInTheDocument()
  })

  it('confirms the Pin meaning before changing the current instance', async () => {
    const setDefault = vi.spyOn(api, 'SetDefault')
    render(<App />)
    fireEvent.click((await screen.findAllByRole('link', { name: /stable-4\.4/ }))[0])
    const pin = await screen.findByRole('button', { name: '设为当前' })
    fireEvent.click(pin)

    expect(setDefault).not.toHaveBeenCalled()
    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveTextContent('普通 godot 命令和 GoDoIt 顶栏默认启动入口')
    fireEvent.click(within(dialog).getByRole('button', { name: '设为当前' }))
    await waitFor(() => expect(setDefault).toHaveBeenCalledWith('stable-4.4'))
    expect(await screen.findByText('已固定为当前启动条目')).toBeInTheDocument()
  })

  it('opens the new instance wizard without crashing', async () => {
    render(<App />)
    fireEvent.click(await screen.findByRole('link', { name: '新建条目' }))

    expect(await screen.findByRole('heading', { name: '配置' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '条目名' })).toBeInTheDocument()
    expect(window.location.hash).toBe('#/instances/new')
  })

  it('prefetches engine and SDK candidates without creating operations', async () => {
    useAppStore.setState({
      engineCandidates: {},
      sdkCandidates: { status: 'idle', items: [], warnings: [] },
      operations: {},
    })
    render(<App />)

    await waitFor(() => expect(useAppStore.getState().engineCandidates['']?.status).toBe('ready'))
    expect(useAppStore.getState().sdkCandidates.status).toBe('ready')
    expect(useAppStore.getState().operations).toEqual({})
  })

  it('does not retry failed candidate reads until explicitly refreshed', async () => {
    useAppStore.setState({
      engineCandidates: {},
      sdkCandidates: { status: 'idle', items: [], warnings: [] },
    })
    const engines = vi.spyOn(api, 'ListAvailableVersions').mockRejectedValue(new Error('engine candidates unavailable'))
    const sdks = vi.spyOn(api, 'ListAvailableSDKs').mockRejectedValue(new Error('SDK candidates unavailable'))
    render(<App />)
    fireEvent.click(await screen.findByRole('link', { name: '新建条目' }))
    fireEvent.click(await screen.findByRole('button', { name: '.NET / C#' }))

    await waitFor(() => {
      expect(useAppStore.getState().engineCandidates['']?.status).toBe('error')
      expect(useAppStore.getState().sdkCandidates.status).toBe('error')
    })
    await new Promise((resolve) => window.setTimeout(resolve, 20))
    expect(engines).toHaveBeenCalledTimes(1)
    expect(sdks).toHaveBeenCalledTimes(1)
  })

  it('keeps candidate warnings and selects engine and SDK channels in two levels', async () => {
    vi.spyOn(api, 'ListAvailableVersions').mockResolvedValue({
      channels: [
        { name: '4.x', versions: [{ version: '4.7.2', editions: ['standard', 'dotnet'], sources: ['godothub'] }] },
        { name: '3.x', versions: [{ version: '3.6.2', editions: ['standard', 'dotnet'], sources: ['github'] }] },
        { name: 'unstable', versions: [{ version: '4.8-dev3', editions: ['standard'], sources: ['godothub'] }] },
      ],
      warnings: [{ source: 'github', message: '版本元数据暂不可用' }],
    })
    vi.spyOn(api, 'ListAvailableSDKs').mockResolvedValue({
      channels: [
        { major_minor: '10.0', phase: 'active', release_type: 'lts', versions: ['10.0.103'] },
        { major_minor: '8.0', phase: 'maintenance', release_type: 'lts', versions: ['8.0.410'] },
      ],
      warnings: [{ message: '使用 SDK 保底列表' }],
    })

    render(<App />)
    fireEvent.click(await screen.findByRole('link', { name: '新建条目' }))
    expect(await screen.findByRole('group', { name: '引擎系列' })).toBeInTheDocument()
    expect(screen.getByText('版本元数据暂不可用')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '.NET / C#' }))
    fireEvent.click(screen.getByRole('button', { name: '3.x' }))
    expect(await screen.findByText('系统 Mono')).toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: 'SDK 通道' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '4.x' }))
    const channel = await screen.findByRole('combobox', { name: 'SDK 通道' })
    expect(channel).toHaveValue('10.0')
    fireEvent.change(channel, { target: { value: '8.0' } })
    expect(screen.getByRole('option', { name: '8.0.410' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: '10.0.103' })).not.toBeInTheDocument()
    expect(screen.getByText('使用 SDK 保底列表')).toBeInTheDocument()
  })

  it('renders doctor checks with status text', async () => {
    window.location.hash = '#/doctor'
    render(<App />)
    expect(await screen.findByRole('heading', { name: 'Doctor' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /正常/ }))
    expect(await screen.findByText('平台 linux/amd64 受支持')).toBeInTheDocument()
    expect(screen.getByText('可以使用，有少量提醒')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^工具/ })).toHaveClass('active')
  })

  it('collects utilities on a standalone tools page', async () => {
    window.location.hash = '#/tools'
    render(<App />)

    expect(await screen.findByRole('heading', { name: '工具' })).toBeInTheDocument()
    const appNavigation = screen.getByRole('navigation', { name: '应用' })
    expect(within(appNavigation).getAllByRole('link')).toHaveLength(3)
    expect(within(appNavigation).getByRole('link', { name: /^工具/ })).toHaveClass('active')
    expect(screen.queryByRole('button', { name: '资源管理' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /分析 Godot 项目/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Doctor/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^引擎/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^\.NET SDK/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^下载来源/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^缓存与孤儿/ })).toBeInTheDocument()
  })

  it('shows bundled legal texts without a network request', async () => {
    window.location.hash = '#/about'
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: '开源许可' }))
    expect(screen.getByText(/GNU AFFERO GENERAL PUBLIC LICENSE/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: '第三方声明' }))
    expect(screen.getByText(/GoDoIt Third-Party Notices/)).toBeInTheDocument()
    expect(screen.getByText(/github\.com\/wailsapp\/wails\/v2 v2\.11\.0/)).toBeInTheDocument()
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('uses the recommended SDK and updates the sidebar before bootstrap refresh', async () => {
    render(<App />)
    fireEvent.click(await screen.findByRole('link', { name: '新建条目' }))
    fireEvent.change(await screen.findByRole('textbox', { name: '条目名' }), { target: { value: 'recommended-sidebar' } })
    fireEvent.click(screen.getByRole('button', { name: '.NET / C#' }))

    const sdk = await screen.findByRole('combobox', { name: 'SDK 版本' })
    expect(sdk).toHaveValue('')
    expect(screen.getByRole('option', { name: '推荐版本（由 core 根据 Godot 版本解析）' })).toBeInTheDocument()

    const next = screen.getByRole('button', { name: '下一步' })
    await waitFor(() => expect(next).toBeEnabled())
    fireEvent.click(next)
    expect(screen.getByText('推荐版本（安装时解析）')).toBeInTheDocument()

    const install = vi.spyOn(api, 'InstallEntry')
    vi.spyOn(api, 'Bootstrap').mockRejectedValueOnce(new Error('injected refresh failure'))
    fireEvent.click(screen.getByRole('button', { name: 'Install' }))

    await waitFor(() => expect(install).toHaveBeenCalled(), { timeout: 2000 })
    expect(install.mock.calls[0][0].sdk_version).toBeUndefined()
    expect(await screen.findByRole('link', { name: /recommended-sidebar/ }, { timeout: 3000 })).toBeInTheDocument()
  })

  it('does not offer or submit an SDK for Standard entries', async () => {
    render(<App />)
    fireEvent.click(await screen.findByRole('link', { name: '新建条目' }))
    fireEvent.change(await screen.findByRole('textbox', { name: '条目名' }), { target: { value: 'standard-no-sdk' } })

    expect(screen.getByText('无需 .NET SDK')).toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: 'SDK 版本' })).not.toBeInTheDocument()

    const next = screen.getByRole('button', { name: '下一步' })
    await waitFor(() => expect(next).toBeEnabled())
    fireEvent.click(next)

    const sdkRow = screen.getByText('.NET SDK').closest('div')
    expect(sdkRow).not.toBeNull()
    expect(within(sdkRow!).getByText('不安装')).toBeInTheDocument()

    const install = vi.spyOn(api, 'InstallEntry')
    fireEvent.click(screen.getByRole('button', { name: 'Install' }))

    await waitFor(() => expect(install).toHaveBeenCalled())
    expect(install.mock.calls[0][0].sdk_strategy).toBeUndefined()
    expect(install.mock.calls[0][0].sdk_version).toBeUndefined()
  })
})
