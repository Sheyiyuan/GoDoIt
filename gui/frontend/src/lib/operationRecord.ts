interface RecordValue { [key: string]: unknown }

function record(value: unknown): RecordValue | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as RecordValue : undefined
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function list(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function assetLines(value: unknown, verb: string): string[] {
  return list(value).flatMap((item) => {
    const asset = record(item)
    const kind = text(asset?.kind)
    const id = text(asset?.id)
    return kind && id ? [`${verb}${assetLabel(kind)}：${id}`] : []
  })
}

function assetLabel(kind: string) {
  return ({ engine: '引擎', sdk: '.NET SDK', template: '导出模板' } as Record<string, string>)[kind] || kind
}

function instanceLines(value: unknown): string[] {
  const instance = record(value)
  if (!instance) return []
  const lines: string[] = []
  if (text(instance.name)) lines.push(`条目：${text(instance.name)}`)
  if (text(instance.engine)) lines.push(`引擎：${text(instance.engine)}`)
  if (text(instance.sdk)) lines.push(`.NET SDK：${text(instance.sdk)}`)
  if (instance.current === true) lines.push('已设为当前条目')
  return lines
}

export function operationResultSummary(operation: string, value: unknown): string[] {
  const result = record(value)
  if (!result) return operation === 'doctor' || operation === 'suggest' ? ['操作已完成'] : []
  switch (operation) {
    case 'install-entry':
      return [...instanceLines(result.instance), ...assetLines(result.installed, '已安装')]
    case 'install-suggestion': {
      const entry = record(result.entry)
      return [...instanceLines(entry?.instance), ...assetLines(entry?.installed, '已安装')]
    }
    case 'remove-instance':
      return [...instanceLines(result.instance).slice(0, 1), ...assetLines(result.orphans, '新增孤儿')]
    case 'autoremove':
      return assetLines(result.removed, '已移除').length ? assetLines(result.removed, '已移除') : ['没有需要移除的孤儿资产']
    case 'attach-template': {
      const template = record(result.template)
      return [...instanceLines(result.instance).slice(0, 1), text(template?.id) ? [`导出模板：${text(template?.id)}`] : [], result.installed === true ? ['模板资产已下载'] : ['已复用现有模板资产']].flat()
    }
    case 'detach-template':
      return [...instanceLines(result.instance).slice(0, 1), ...assetLines(result.orphans, '新增孤儿'), '已解除导出模板绑定']
    case 'set-instance-icon':
      return [...instanceLines(value).slice(0, 1), '条目图标已更新']
    case 'doctor': {
      const errors = typeof result.error_count === 'number' ? result.error_count : 0
      const warnings = typeof result.warn_count === 'number' ? result.warn_count : 0
      return [`诊断完成：${errors} 项错误，${warnings} 项警告`]
    }
    case 'suggest':
      return [text(result.engine_series) ? `建议 Godot ${text(result.engine_series)} · ${text(result.edition) || 'standard'}` : '项目分析已完成']
    default:
      return ['操作已完成']
  }
}

export function operationRoute(operation: string): string {
  if (operation === 'doctor') return '/doctor'
  if (operation === 'suggest' || operation === 'install-suggestion') return '/suggest'
  if (operation === 'install-entry') return '/instances/new'
  if (operation === 'autoremove') return '/resources/cache'
  return '/instances'
}

export function operationLabel(operation: string) {
  return ({
    'install-entry': '安装条目',
    'install-suggestion': '安装项目建议',
    'remove-instance': '卸载条目',
    'attach-template': '下载导出模板',
    'detach-template': '解除模板绑定',
    'set-instance-icon': '更新条目图标',
    autoremove: '清理孤儿资产',
    doctor: '运行诊断',
    suggest: '分析项目',
  } as Record<string, string>)[operation] || operation
}

export function sanitizeOperationText(value: string | undefined): string | undefined {
  if (!value) return value
  return value
    .replace(/https?:\/\/[^\s)\]}]+/gi, '[链接]')
    .replace(/\b(token|secret|password|api[_-]?key)\s*[:=]\s*[^\s,;]+/gi, '$1=[已隐藏]')
    .replace(/(?:[A-Za-z]:\\|\/)(?:[^\s:]+[\\/])+[^\s:]+/g, '[路径]')
}
