import { X } from 'lucide-react'
import { EnvironmentEditor } from './EnvironmentEditor'

export function EnvironmentPanel({ name, open, onClose, onChanged }: { name: string; open: boolean; onClose: () => void; onChanged?: () => void }) {
  if (!open) return null
  return <div className="panel-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><section className="side-panel environment-panel" role="dialog" aria-modal="true" aria-labelledby="environment-panel-title"><header className="side-panel-header"><div><strong id="environment-panel-title">运行环境</strong><small>{name}</small></div><button className="icon-button" type="button" aria-label="关闭环境变量" onClick={onClose}><X /></button></header><EnvironmentEditor name={name} editableScope="instance" onChanged={onChanged} /></section></div>
}
