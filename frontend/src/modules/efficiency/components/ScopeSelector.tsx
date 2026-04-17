import { useEffect, useMemo, useState } from 'react'
import type { ExtensionScope } from '../types'
import { fetchBrowserProfiles } from '../../browser/api'
import type { BrowserProfile, BrowserGroupWithCount } from '../../browser/types'

interface Props {
  value: ExtensionScope
  onChange: (next: ExtensionScope) => void
}

async function listGroups(): Promise<BrowserGroupWithCount[]> {
  try {
    const bindings: any = await import('../../../wailsjs/go/main/App')
    if (bindings?.ListGroups) return (await bindings.ListGroups()) || []
  } catch {}
  return []
}

export function ScopeSelector({ value, onChange }: Props) {
  const [mode, setMode] = useState<'instances' | 'groups'>(value.kind)
  const [profiles, setProfiles] = useState<BrowserProfile[]>([])
  const [groups, setGroups] = useState<BrowserGroupWithCount[]>([])
  const [query, setQuery] = useState('')
  const [instancesDraft, setInstancesDraft] = useState<string[]>(value.kind === 'instances' ? value.ids : [])
  const [groupsDraft, setGroupsDraft] = useState<string[]>(value.kind === 'groups' ? value.ids : [])

  useEffect(() => {
    fetchBrowserProfiles().then(setProfiles).catch(() => setProfiles([]))
    listGroups().then(setGroups).catch(() => setGroups([]))
  }, [])

  // Emit the current mode's draft whenever user switches modes or updates selection.
  // Pushed synchronously from the event handlers below rather than via an effect
  // to avoid an onChange identity loop.
  const emit = (nextMode: 'instances' | 'groups', nextInstances: string[], nextGroups: string[]) => {
    if (nextMode === 'instances') onChange({ kind: 'instances', ids: nextInstances })
    else onChange({ kind: 'groups', ids: nextGroups })
  }

  const switchMode = (m: 'instances' | 'groups') => {
    setMode(m)
    emit(m, instancesDraft, groupsDraft)
  }

  const toggleInstance = (id: string) => {
    const next = instancesDraft.includes(id)
      ? instancesDraft.filter((x) => x !== id)
      : [...instancesDraft, id]
    setInstancesDraft(next)
    emit(mode, next, groupsDraft)
  }

  const toggleGroup = (id: string) => {
    const next = groupsDraft.includes(id)
      ? groupsDraft.filter((x) => x !== id)
      : [...groupsDraft, id]
    setGroupsDraft(next)
    emit(mode, instancesDraft, next)
  }

  const filteredProfiles = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? profiles.filter((p) => p.profileName.toLowerCase().includes(q)) : profiles
  }, [profiles, query])

  const effectiveCount = useMemo(() => {
    if (mode === 'instances') return instancesDraft.length
    const sel = new Set(groupsDraft)
    return groups.filter((g) => sel.has(g.groupId)).reduce((acc, g) => acc + (g.instanceCount || 0), 0)
  }, [mode, instancesDraft, groupsDraft, groups])

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-4 text-sm">
        <label className="inline-flex items-center gap-1 cursor-pointer">
          <input type="radio" checked={mode === 'groups'} onChange={() => switchMode('groups')} />
          按分组
        </label>
        <label className="inline-flex items-center gap-1 cursor-pointer">
          <input type="radio" checked={mode === 'instances'} onChange={() => switchMode('instances')} />
          按实例
        </label>
      </div>

      {mode === 'instances' && (
        <>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索实例名..."
            className="h-9 w-full rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-3 text-sm"
          />
          <div className="max-h-64 overflow-y-auto border border-[var(--color-border-muted)] rounded-md divide-y divide-[var(--color-border-muted)]">
            {filteredProfiles.map((p) => (
              <label key={p.profileId} className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-[var(--color-bg-muted)]">
                <input
                  type="checkbox"
                  checked={instancesDraft.includes(p.profileId)}
                  onChange={() => toggleInstance(p.profileId)}
                />
                <span className="truncate">{p.profileName}</span>
              </label>
            ))}
            {filteredProfiles.length === 0 && <div className="p-3 text-xs text-[var(--color-text-muted)]">没有匹配的实例</div>}
          </div>
        </>
      )}

      {mode === 'groups' && (
        <div className="max-h-64 overflow-y-auto border border-[var(--color-border-muted)] rounded-md divide-y divide-[var(--color-border-muted)]">
          {groups.map((g) => (
            <label key={g.groupId} className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-[var(--color-bg-muted)]">
              <input
                type="checkbox"
                checked={groupsDraft.includes(g.groupId)}
                onChange={() => toggleGroup(g.groupId)}
              />
              <span className="truncate">{g.groupName}</span>
              <span className="ml-auto text-xs text-[var(--color-text-muted)]">{g.instanceCount} 个实例</span>
            </label>
          ))}
          {groups.length === 0 && <div className="p-3 text-xs text-[var(--color-text-muted)]">暂无分组</div>}
        </div>
      )}

      <div className="text-xs text-[var(--color-text-muted)]">将在 {effectiveCount} 个实例上生效</div>
    </div>
  )
}
