import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown, X } from 'lucide-react'
import type { ExtensionScope } from '../types'
import { fetchBrowserProfiles } from '../../browser/api'
import type { BrowserProfile, BrowserGroupWithCount } from '../../browser/types'

interface Props {
  value: ExtensionScope
  onChange: (next: ExtensionScope) => void
}

type UIMode = 'all' | 'groups' | 'instances'

async function listGroups(): Promise<BrowserGroupWithCount[]> {
  try {
    const bindings: any = await import('../../../wailsjs/go/main/App')
    if (bindings?.ListGroups) return (await bindings.ListGroups()) || []
  } catch {}
  return []
}

interface Option {
  value: string
  label: string
  tag?: string
}

function MultiSelectDropdown({
  placeholder,
  options,
  selected,
  onChange,
  emptyHint,
}: {
  placeholder: string
  options: Option[]
  selected: string[]
  onChange: (next: string[]) => void
  emptyHint?: string
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [panelStyle, setPanelStyle] = useState<CSSProperties>({})

  const syncPanel = () => {
    const r = triggerRef.current?.getBoundingClientRect()
    if (!r) return
    // Fixed-position panel anchored to the trigger. Portalled to body so it
    // isn't clipped by ancestor overflow containers (e.g., the modal body).
    setPanelStyle({
      position: 'fixed',
      top: r.bottom + 4,
      left: r.left,
      width: r.width,
      zIndex: 60,
    })
  }

  useLayoutEffect(() => {
    if (open) syncPanel()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onScroll = () => syncPanel()
    const onResize = () => syncPanel()
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (triggerRef.current?.contains(t)) return
      if (panelRef.current?.contains(t)) return
      setOpen(false)
    }
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onResize)
    document.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onResize)
      document.removeEventListener('mousedown', onDown)
    }
  }, [open])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? options.filter((o) => o.label.toLowerCase().includes(q)) : options
  }, [options, query])

  const selectedSet = useMemo(() => new Set(selected), [selected])
  const toggle = (v: string) => {
    onChange(selectedSet.has(v) ? selected.filter((x) => x !== v) : [...selected, v])
  }

  return (
    <div className="relative">
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="h-9 w-full rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-3 text-sm flex items-center justify-between gap-2"
      >
        {selected.length === 0
          ? <span className="text-[var(--color-text-muted)]">{placeholder}</span>
          : <span className="truncate">已选 {selected.length} 项</span>}
        <ChevronDown className={`w-4 h-4 text-[var(--color-text-muted)] transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>

      {selected.length > 0 && (
        <div className="flex flex-wrap gap-1 mt-2">
          {selected.map((id) => {
            const opt = options.find((o) => o.value === id)
            return (
              <span key={id} className="inline-flex items-center gap-1 rounded bg-[var(--color-bg-muted)] px-2 py-0.5 text-xs">
                <span className="truncate max-w-[160px]">{opt?.label ?? id}</span>
                <button
                  type="button"
                  onClick={() => toggle(id)}
                  className="text-[var(--color-text-muted)] hover:text-rose-500"
                >
                  <X className="w-3 h-3" />
                </button>
              </span>
            )
          })}
        </div>
      )}

      {open && createPortal(
        <div
          ref={panelRef}
          style={panelStyle}
          className="rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] shadow-md"
        >
          <div className="p-2 border-b border-[var(--color-border-muted)]">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索..."
              className="h-8 w-full rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-2 text-sm"
            />
          </div>
          <div className="max-h-56 overflow-y-auto">
            {filtered.map((opt) => (
              <label
                key={opt.value}
                className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-[var(--color-bg-muted)]"
              >
                <input
                  type="checkbox"
                  checked={selectedSet.has(opt.value)}
                  onChange={() => toggle(opt.value)}
                />
                <span className="truncate flex-1">{opt.label}</span>
                {opt.tag && <span className="text-xs text-[var(--color-text-muted)]">{opt.tag}</span>}
              </label>
            ))}
            {filtered.length === 0 && (
              <div className="p-3 text-xs text-[var(--color-text-muted)]">{emptyHint || '暂无数据'}</div>
            )}
          </div>
        </div>,
        document.body,
      )}
    </div>
  )
}

export function ScopeSelector({ value, onChange }: Props) {
  const [mode, setMode] = useState<UIMode>(value.kind)
  const [profiles, setProfiles] = useState<BrowserProfile[]>([])
  const [groups, setGroups] = useState<BrowserGroupWithCount[]>([])
  const [instancesDraft, setInstancesDraft] = useState<string[]>(value.kind === 'instances' ? value.ids : [])
  const [groupsDraft, setGroupsDraft] = useState<string[]>(value.kind === 'groups' ? value.ids : [])

  useEffect(() => {
    fetchBrowserProfiles().then(setProfiles).catch(() => setProfiles([]))
    listGroups().then(setGroups).catch(() => setGroups([]))
  }, [])

  const emit = (nextMode: UIMode, nextInstances: string[], nextGroups: string[]) => {
    if (nextMode === 'all') onChange({ kind: 'all', ids: [] })
    else if (nextMode === 'instances') onChange({ kind: 'instances', ids: nextInstances })
    else onChange({ kind: 'groups', ids: nextGroups })
  }

  const switchMode = (m: UIMode) => {
    setMode(m)
    emit(m, instancesDraft, groupsDraft)
  }

  const effectiveCount = useMemo(() => {
    if (mode === 'all') return profiles.length
    if (mode === 'instances') return instancesDraft.length
    const sel = new Set(groupsDraft)
    return groups.filter((g) => sel.has(g.groupId)).reduce((acc, g) => acc + (g.instanceCount || 0), 0)
  }, [mode, profiles.length, instancesDraft, groupsDraft, groups])

  const instanceOptions = useMemo<Option[]>(
    () => profiles.map((p) => ({ value: p.profileId, label: p.profileName })),
    [profiles],
  )
  const groupOptions = useMemo<Option[]>(
    () => groups.map((g) => ({ value: g.groupId, label: g.groupName, tag: `${g.instanceCount} 个实例` })),
    [groups],
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-4 text-sm">
        <label className="inline-flex items-center gap-1 cursor-pointer">
          <input type="radio" checked={mode === 'all'} onChange={() => switchMode('all')} />
          全部
        </label>
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
        <MultiSelectDropdown
          placeholder="选择实例..."
          options={instanceOptions}
          selected={instancesDraft}
          onChange={(next) => {
            setInstancesDraft(next)
            emit('instances', next, groupsDraft)
          }}
          emptyHint="没有匹配的实例"
        />
      )}

      {mode === 'groups' && (
        <MultiSelectDropdown
          placeholder="选择分组..."
          options={groupOptions}
          selected={groupsDraft}
          onChange={(next) => {
            setGroupsDraft(next)
            emit('groups', instancesDraft, next)
          }}
          emptyHint="暂无分组"
        />
      )}

      <div className="text-xs text-[var(--color-text-muted)]">将在 {effectiveCount} 个实例上生效</div>
    </div>
  )
}
