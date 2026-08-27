import type { TitlebarStyle } from '../types'

type WindowPlatform = 'windows' | 'other'

function runtimeAvailable() {
  return window.runtime
}

export function minimiseWindow() {
  runtimeAvailable()?.WindowMinimise?.()
}

export function toggleMaximiseWindow() {
  runtimeAvailable()?.WindowToggleMaximise?.()
}

export function closeWindow() {
  runtimeAvailable()?.Quit?.()
}

export async function isWindowMaximised() {
  return Boolean(await runtimeAvailable()?.WindowIsMaximised?.())
}

export async function detectWindowPlatform(): Promise<WindowPlatform> {
  const environment = runtimeAvailable()?.Environment
  if (environment) {
    try {
      const info = await environment()
      return info.platform === 'windows' ? 'windows' : 'other'
    } catch {
      // 浏览器预览或旧版 runtime 不提供 Environment 时回退到浏览器平台提示。
    }
  }
  return /win/i.test(window.navigator.platform) ? 'windows' : 'other'
}

export async function resolveTitlebarStyle(preference: TitlebarStyle): Promise<Exclude<TitlebarStyle, 'auto'>> {
  if (preference !== 'auto') return preference
  return (await detectWindowPlatform()) === 'windows' ? 'windows' : 'mac'
}
