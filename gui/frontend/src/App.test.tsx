import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import App from './App'

afterEach(() => {
  cleanup()
  window.location.hash = ''
})

describe('GoDoIt workbench', () => {
  it('puts the create action before instances and opens the current instance', async () => {
    render(<App />)
    const create = await screen.findByRole('link', { name: '新建条目' })
    const instanceLinks = await screen.findAllByRole('link', { name: /studio-csharp/ })
    expect(create.compareDocumentPosition(instanceLinks[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    await waitFor(() => expect(screen.getByRole('heading', { name: 'studio-csharp' })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: '启动 Godot' })).toBeEnabled()
  })

  it('renders doctor checks with status text', async () => {
    window.location.hash = '#/doctor'
    render(<App />)
    expect(await screen.findByRole('heading', { name: 'Doctor' })).toBeInTheDocument()
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
})
