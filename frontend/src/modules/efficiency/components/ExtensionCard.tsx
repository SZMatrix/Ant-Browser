import { Settings, Trash2 } from 'lucide-react'
import type { ExtensionView } from '../types'
import { Switch } from '../../../shared/components'
import placeholder from '../../../resources/images/extension-placeholder.svg'

interface Props {
  data: ExtensionView
  cdpSupported?: boolean
  profileNames?: Record<string, string>
  groupNames?: Record<string, string>
  onToggle: (id: string, enabled: boolean) => void
  onDelete: (id: string) => void
  onChanged: () => void
}

export function ExtensionCard({ data, cdpSupported, profileNames, groupNames, onToggle, onDelete }: Props) {
  const scopeLabel = data.scope.kind === 'groups' ? '分组' : '实例'
  const nameMap = data.scope.kind === 'groups' ? groupNames : profileNames
  const resolvedNames = data.scope.ids.map((id) => nameMap?.[id] ?? id)
  const scopeSummary = resolvedNames.length === 0
    ? `${scopeLabel}：未指定`
    : `${scopeLabel}：${resolvedNames.join('、')}`

  const pendingCount = data.pendingRestartProfileIds.length

  return (
    <div className={`rounded-lg border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] p-4 flex flex-col gap-3 transition-opacity ${data.enabled ? '' : 'opacity-60'}`}>
      <div className="flex items-start gap-3">
        <div className="w-12 h-12 rounded bg-[var(--color-bg-muted)] overflow-hidden flex items-center justify-center flex-shrink-0">
          <img
            src={data.iconDataURL || placeholder}
            alt=""
            className="w-full h-full object-contain"
            onError={(e) => { (e.currentTarget as HTMLImageElement).src = placeholder }}
          />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium text-[var(--color-text-primary)] truncate">{data.name}</h3>
            <div className="flex items-center gap-1">
              <button title="编辑" className="p-1 rounded hover:bg-[var(--color-bg-muted)]">
                <Settings className="w-4 h-4 text-[var(--color-text-muted)]" />
              </button>
              <button title="删除" className="p-1 rounded hover:bg-[var(--color-bg-muted)]" onClick={() => onDelete(data.extensionId)}>
                <Trash2 className="w-4 h-4 text-[var(--color-text-muted)]" />
              </button>
            </div>
          </div>
          <div className="flex items-center justify-between gap-2 mt-1">
            <p className="text-xs text-[var(--color-text-muted)] truncate">by {data.provider || '未知'} · v{data.version || '—'}</p>
            <Switch checked={data.enabled} onChange={(v) => onToggle(data.extensionId, v)} />
          </div>
        </div>
      </div>

      <p
        className="text-xs text-[var(--color-text-secondary)] line-clamp-2 overflow-hidden min-h-8"
        title={data.description || '（无描述）'}
      >
        {data.description || '（无描述）'}
      </p>

      <div className="border-t border-[var(--color-border-muted)] pt-3 text-xs">
        <span
          className="block text-[var(--color-text-muted)] truncate"
          title={scopeSummary}
        >
          范围：{scopeSummary}
        </span>
      </div>

      {pendingCount > 0 && (
        <div className="text-xs text-amber-600" title={data.pendingRestartProfileIds.join(', ')}>
          {cdpSupported === false
            ? `当前内核不支持实时生效，请重启这 ${pendingCount} 个实例`
            : `🔄 ${pendingCount} 个实例注入失败，请重启生效`}
        </div>
      )}
      {data.staleScopeIds.length > 0 && (
        <div className="text-xs text-rose-500" title={data.staleScopeIds.join(', ')}>
          ⚠ {data.staleScopeIds.length} 个范围 ID 已失效
        </div>
      )}
    </div>
  )
}
