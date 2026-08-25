import { LoaderCircle, X } from 'lucide-react'
import { api } from '../lib/api'
import { useAppStore } from '../store/app'

export function OperationTray() {
  const operations = useAppStore((state) => state.operations)
  const running = Object.values(operations).filter((event) => event.status === 'running')
  if (running.length === 0) return null
  return (
    <aside className="operation-tray" aria-live="polite">
      {running.map((event) => (
        <div className="operation-row" key={event.operation_id}>
          <LoaderCircle className="spin" aria-hidden="true" />
          <span><strong>{operationLabel(event.operation)}</strong><small>{event.progress?.message || event.progress?.stage || '处理中'}</small></span>
          <button className="icon-button" type="button" title="取消任务" aria-label={`取消${operationLabel(event.operation)}`} onClick={() => void api.Cancel(event.operation_id)}><X /></button>
        </div>
      ))}
    </aside>
  )
}

function operationLabel(operation: string) {
  return ({
    'available-versions': '读取引擎版本',
    'available-sdks': '读取 SDK 版本',
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
