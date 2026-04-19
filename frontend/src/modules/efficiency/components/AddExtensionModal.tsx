import { useState } from 'react'
import { Button, Modal } from '../../../shared/components'
import {
  createInstalling,
  getExtension,
  identifyFromLocal,
  identifyFromStore,
  renameExtension,
  updateScope,
} from '../api'
import type { ExtensionMetadata, ExtensionScope, ExtensionView } from '../types'
import { ScopeSelector } from './ScopeSelector'
import placeholder from '../../../resources/images/extension-placeholder.svg'

interface Props {
  open: boolean
  onClose: () => void
  onCommitted: () => void
  // When provided, the modal opens in edit mode: no source tabs / identify UI,
  // just the preview + scope editor for the given extension.
  editing?: ExtensionView | null
}

type Tab = 'store' | 'local'
type Phase = 'idle' | 'identifying' | 'identified'

// Build the preview-shaped metadata from an existing extension so the
// identified-preview section can be reused for edit mode.
function metaFromExtension(ext: ExtensionView): ExtensionMetadata {
  return {
    chromeId: ext.chromeId,
    name: ext.name,
    provider: ext.provider,
    description: ext.description,
    version: ext.version,
    iconDataURL: ext.iconDataURL,
    sourceType: ext.sourceType,
    storeVendor: ext.storeVendor,
    sourceUrl: ext.sourceUrl,
    duplicateOf: '',
  }
}

export function AddExtensionModal({ open, onClose, onCommitted, editing }: Props) {
  const isEdit = !!editing
  const [tab, setTab] = useState<Tab>('store')
  const [phase, setPhase] = useState<Phase>(isEdit ? 'identified' : 'idle')
  const [error, setError] = useState('')
  const [storeURL, setStoreURL] = useState('')
  const [meta, setMeta] = useState<ExtensionMetadata | null>(
    isEdit && editing ? metaFromExtension(editing) : null,
  )
  const [overrideName, setOverrideName] = useState(isEdit && editing ? editing.name : '')
  const [scope, setScope] = useState<ExtensionScope>(
    isEdit && editing ? editing.scope : { kind: 'instances', ids: [] },
  )
  const [submitting, setSubmitting] = useState(false)

  const tabBtn = (active: boolean) =>
    `px-3 py-2 text-sm ${active ? 'text-[var(--color-text-primary)] border-b-2 border-[var(--color-accent)]' : 'text-[var(--color-text-muted)]'}`

  const resetToIdle = () => {
    setPhase('idle')
    setMeta(null)
    setError('')
    setOverrideName('')
    setScope({ kind: 'instances', ids: [] })
  }

  const switchTab = (next: Tab) => {
    setTab(next)
    resetToIdle()
    setStoreURL('')
  }

  const applyIdentified = async (m: ExtensionMetadata) => {
    setMeta(m)
    // When the identified extension is a duplicate, pre-fill scope + name from
    // the existing row so the user edits the existing install instead of
    // starting from a blank scope. Keep store-provided name/scope defaults
    // when it's a fresh install.
    if (m.duplicateOf) {
      const existing = await getExtension(m.duplicateOf)
      if (existing) {
        setOverrideName(existing.name || m.name)
        setScope(existing.scope || { kind: 'instances', ids: [] })
      } else {
        setOverrideName(m.name)
        setScope({ kind: 'instances', ids: [] })
      }
    } else {
      setOverrideName(m.name)
      setScope({ kind: 'instances', ids: [] })
    }
    setPhase('identified')
  }

  const runIdentifyStore = async () => {
    setError('')
    setPhase('identifying')
    try {
      const m = await identifyFromStore(storeURL.trim())
      if (!m) { setPhase('idle'); return }
      await applyIdentified(m)
    } catch (e: any) {
      setError(e?.message || '识别失败')
      setPhase('idle')
    }
  }

  const runIdentifyLocal = async () => {
    setError('')
    setPhase('identifying')
    try {
      const m = await identifyFromLocal('')
      if (!m) { setPhase('idle'); return }
      await applyIdentified(m)
    } catch (e: any) {
      setError(e?.message || '识别失败')
      setPhase('idle')
    }
  }

  const handleAdd = async () => {
    if (!meta) return
    setSubmitting(true)
    setError('')
    try {
      await createInstalling(meta, scope, overrideName)
      onCommitted()
      resetToIdle()
      setStoreURL('')
    } catch (e: any) {
      setError(e?.message || '添加失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleSaveEdit = async () => {
    if (!editing) return
    setSubmitting(true)
    setError('')
    try {
      const name = overrideName.trim()
      if (name && name !== editing.name) {
        await renameExtension(editing.extensionId, name)
      }
      await updateScope(editing.extensionId, scope)
      onCommitted()
    } catch (e: any) {
      setError(e?.message || '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleClose = () => {
    if (!isEdit) {
      resetToIdle()
      setStoreURL('')
    }
    onClose()
  }

  return (
    <Modal open={open} onClose={handleClose} title={isEdit ? '编辑扩展' : '添加扩展'} width="560px">
      <div className="p-4 space-y-4">
        {error && <div className="text-sm text-rose-500">{error}</div>}

        {!isEdit && (
          <>
            <div className="flex border-b border-[var(--color-border-muted)]">
              <button className={tabBtn(tab === 'store')} onClick={() => switchTab('store')}>从扩展商店</button>
              <button className={tabBtn(tab === 'local')} onClick={() => switchTab('local')}>从本地文件</button>
            </div>

            {tab === 'store' ? (
              <div className="flex items-center gap-2">
                <input
                  value={storeURL}
                  onChange={(e) => {
                    setStoreURL(e.target.value)
                    if (phase !== 'idle') resetToIdle()
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && storeURL.trim() && phase !== 'identifying') {
                      e.preventDefault()
                      runIdentifyStore()
                    }
                  }}
                  placeholder="粘贴 Chrome / Edge 商店详情页 URL，回车识别"
                  className="h-9 flex-1 rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-3 text-sm"
                />
                <Button
                  onClick={runIdentifyStore}
                  loading={phase === 'identifying'}
                  disabled={!storeURL.trim() || phase === 'identifying'}
                >
                  识别
                </Button>
              </div>
            ) : (
              <div className="space-y-2">
                <p className="text-sm text-[var(--color-text-secondary)]">选择 <code>.crx</code> 或 <code>.zip</code> 文件。识别会在选择后自动进行。</p>
                <Button onClick={runIdentifyLocal} loading={phase === 'identifying'}>选择文件...</Button>
              </div>
            )}
          </>
        )}

        {phase === 'identified' && meta && (
          <>
            <div className="flex items-start gap-3">
              <img
                src={meta.iconDataURL || placeholder}
                alt=""
                className="w-12 h-12 rounded object-contain bg-[var(--color-bg-muted)]"
              />
              <div className="flex-1">
                <input
                  className="h-8 w-full rounded border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-2 text-sm font-medium"
                  value={overrideName}
                  onChange={(e) => setOverrideName(e.target.value)}
                />
                <p className="text-xs text-[var(--color-text-muted)] mt-1">
                  提供方：{meta.provider || '未知'} · v{meta.version || '—'}
                </p>
              </div>
            </div>

            {!isEdit && meta.duplicateOf && (
              <div className="rounded bg-amber-50 text-amber-700 px-3 py-2 text-xs">
                检测到已安装该扩展，提交后将覆盖更新现有插件（范围、名称一并替换）。
              </div>
            )}

            <ScopeSelector value={scope} onChange={setScope} />

            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={handleClose}>取消</Button>
              {isEdit ? (
                <Button onClick={handleSaveEdit} loading={submitting}>保存</Button>
              ) : (
                <Button onClick={handleAdd} loading={submitting}>
                  {meta.duplicateOf ? '覆盖更新' : '添加'}
                </Button>
              )}
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}
