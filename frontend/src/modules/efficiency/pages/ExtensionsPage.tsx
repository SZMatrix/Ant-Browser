import { useCallback, useEffect, useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '../../../shared/components'
import { listExtensions, deleteExtension, setEnabled } from '../api'
import type { ExtensionView } from '../types'
import { ExtensionCard } from '../components/ExtensionCard'
import { AddExtensionModal } from '../components/AddExtensionModal'
import { fetchBrowserProfiles } from '../../browser/api'

async function fetchGroups(): Promise<Array<{ groupId: string; groupName: string }>> {
  try {
    const bindings: any = await import('../../../wailsjs/go/main/App')
    if (bindings?.ListGroups) return (await bindings.ListGroups()) || []
  } catch {}
  return []
}

export function ExtensionsPage() {
  const [exts, setExts] = useState<ExtensionView[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [addOpen, setAddOpen] = useState(false)
  const [lastCDPSupport, setLastCDPSupport] = useState<Record<string, boolean>>({})
  const [profileNames, setProfileNames] = useState<Record<string, string>>({})
  const [groupNames, setGroupNames] = useState<Record<string, string>>({})

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [list, profiles, groups] = await Promise.all([
        listExtensions(),
        fetchBrowserProfiles().catch(() => []),
        fetchGroups().catch(() => []),
      ])
      setExts(list)
      setProfileNames(Object.fromEntries(profiles.map((p) => [p.profileId, p.profileName])))
      setGroupNames(Object.fromEntries(groups.map((g) => [g.groupId, g.groupName])))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { reload() }, [reload])

  useEffect(() => {
    const runtime = (window as any).runtime
    if (!runtime?.EventsOn) return
    runtime.EventsOn('extensions:changed', () => { reload() })
    return () => runtime.EventsOff?.('extensions:changed')
  }, [reload])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return exts
    return exts.filter((e) =>
      e.name.toLowerCase().includes(q)
      || e.provider.toLowerCase().includes(q)
      || e.description.toLowerCase().includes(q),
    )
  }, [exts, query])

  const handleToggle = async (id: string, enabled: boolean) => {
    const res = await setEnabled(id, enabled)
    setLastCDPSupport((prev) => ({ ...prev, [id]: res.cdpSupportedByKernel }))
    await reload()
  }

  const handleDelete = async (id: string) => {
    if (!confirm('删除后磁盘上的扩展目录会被清理，确定继续？')) return
    await deleteExtension(id)
    await reload()
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold text-[var(--color-text-primary)]">扩展</h1>
        <div className="flex items-center gap-3">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索名称 / 提供方 / 描述"
            className="h-9 w-64 rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-3 text-sm"
          />
          <Button onClick={() => setAddOpen(true)}>
            <Plus className="w-4 h-4 mr-1" /> 添加扩展
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-4">
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-44 rounded-lg bg-[var(--color-bg-muted)] animate-pulse" />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-sm text-[var(--color-text-muted)]">
          {query.trim()
            ? '没有匹配的扩展'
            : '暂无扩展，点击右上角「添加扩展」'}
        </div>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-4">
          {filtered.map((ext) => (
            <ExtensionCard
              key={ext.extensionId}
              data={ext}
              cdpSupported={lastCDPSupport[ext.extensionId]}
              profileNames={profileNames}
              groupNames={groupNames}
              onToggle={handleToggle}
              onDelete={handleDelete}
              onChanged={reload}
            />
          ))}
        </div>
      )}

      {addOpen && (
        <AddExtensionModal
          open={addOpen}
          onClose={() => setAddOpen(false)}
          onCommitted={async () => {
            setAddOpen(false)
            await reload()
          }}
        />
      )}
    </div>
  )
}
