import { AlertCircle, Loader2, Pencil, Trash2 } from 'lucide-react'
import type { ExtensionView } from '../types'
import { Switch } from '../../../shared/components'
import { retryInstall } from '../api'
import placeholder from '../../../resources/images/extension-placeholder.svg'

interface Props {
  data: ExtensionView
  profileNames?: Record<string, string>
  groupNames?: Record<string, string>
  onToggle: (id: string, enabled: boolean) => void
  onDelete: (ext: ExtensionView) => void
  onEdit: (ext: ExtensionView) => void
  onChanged: () => void
}

export function ExtensionCard({ data, profileNames, groupNames, onToggle, onDelete, onEdit, onChanged }: Props) {
  const status = data.installStatus || 'succeeded'
  const isInstalling = status === 'installing'
  const isFailed = status === 'failed'

  let scopeSummary: string
  if (data.scope.kind === 'all') {
    scopeSummary = '全部：所有实例（含后续新增）'
  } else {
    const scopeLabel = data.scope.kind === 'groups' ? '分组' : '实例'
    const nameMap = data.scope.kind === 'groups' ? groupNames : profileNames
    const resolvedNames = data.scope.ids.map((id) => nameMap?.[id] ?? id)
    scopeSummary = resolvedNames.length === 0
      ? `${scopeLabel}：未指定`
      : `${scopeLabel}：${resolvedNames.join('、')}`
  }

  const handleRetry = async () => {
    try {
      await retryInstall(data.extensionId)
      onChanged()
    } catch {
      // Swallow — listener on the page will re-fetch and surface any
      // persisted error via the tooltip on next render.
    }
  }

  const wrapperClass = [
    'rounded-lg border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] p-4 flex flex-col gap-3 transition-opacity',
    isInstalling ? 'opacity-60' : (data.enabled ? '' : 'opacity-60'),
  ].join(' ')

  return (
    <div className={wrapperClass}>
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
            <div className="flex items-center gap-1 min-w-0">
              <h3 className="text-sm font-medium text-[var(--color-text-primary)] truncate">{data.name}</h3>
              {isInstalling && <Loader2 className="w-4 h-4 text-[var(--color-text-muted)] animate-spin flex-shrink-0" />}
              {isFailed && (
                <button
                  type="button"
                  title={data.installError || '安装失败，点击重试'}
                  onClick={handleRetry}
                  className="p-0.5 rounded hover:bg-[var(--color-bg-muted)] flex-shrink-0"
                >
                  <AlertCircle className="w-4 h-4 text-rose-500" />
                </button>
              )}
            </div>
            <div className="flex items-center gap-1">
              <button
                title="删除"
                disabled={isInstalling}
                onClick={() => !isInstalling && onDelete(data)}
                className="p-1 rounded hover:bg-[var(--color-bg-muted)] disabled:opacity-40 disabled:cursor-not-allowed"
              >
                <Trash2 className="w-4 h-4 text-[var(--color-text-muted)]" />
              </button>
            </div>
          </div>
          <div className="flex items-center justify-between gap-2 mt-1">
            <p className="text-xs text-[var(--color-text-muted)] truncate">by {data.provider || '未知'} · v{data.version || '—'}</p>
            <Switch
              checked={data.enabled}
              disabled={isInstalling || isFailed}
              onChange={(v) => onToggle(data.extensionId, v)}
            />
          </div>
        </div>
      </div>

      <p
        className="text-xs text-[var(--color-text-secondary)] line-clamp-2 overflow-hidden min-h-8"
        title={data.description || '（无描述）'}
      >
        {data.description || '（无描述）'}
      </p>

      <div className="border-t border-[var(--color-border-muted)] pt-3 text-xs flex items-center gap-2">
        <span
          className="flex-1 min-w-0 text-[var(--color-text-muted)] truncate"
          title={scopeSummary}
        >
          范围：{scopeSummary}
        </span>
        <button
          title="编辑"
          disabled={isInstalling || isFailed}
          onClick={() => onEdit(data)}
          className="p-1 rounded hover:bg-[var(--color-bg-muted)] disabled:opacity-40 disabled:cursor-not-allowed flex-shrink-0"
        >
          <Pencil className="w-4 h-4 text-[var(--color-text-muted)]" />
        </button>
      </div>

      {data.staleScopeIds.length > 0 && !isInstalling && !isFailed && (
        <div className="text-xs text-rose-500" title={data.staleScopeIds.join(', ')}>
          ⚠ {data.staleScopeIds.length} 个范围 ID 已失效
        </div>
      )}
    </div>
  )
}
