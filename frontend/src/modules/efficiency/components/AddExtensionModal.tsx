import { useEffect, useState } from 'react'
import { Button, Modal } from '../../../shared/components'
import { cancelPreview, commitExtension, previewFromLocal, previewFromStore } from '../api'
import type { ExtensionPreview, ExtensionScope } from '../types'
import { ScopeSelector } from './ScopeSelector'
import placeholder from '../../../resources/images/extension-placeholder.svg'

interface Props {
  open: boolean
  onClose: () => void
  onCommitted: () => void
}

type Step = 'choose' | 'scope'

export function AddExtensionModal({ open, onClose, onCommitted }: Props) {
  const [step, setStep] = useState<Step>('choose')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<ExtensionPreview | null>(null)
  const [scope, setScope] = useState<ExtensionScope>({ kind: 'instances', ids: [] })
  const [overrideName, setOverrideName] = useState('')
  const [activeTab, setActiveTab] = useState<'store' | 'local'>('store')
  const [storeURL, setStoreURL] = useState('')

  const tabBtn = (active: boolean) =>
    `px-3 py-2 text-sm ${active ? 'text-[var(--color-text-primary)] border-b-2 border-[var(--color-accent)]' : 'text-[var(--color-text-muted)]'}`

  const pickStore = async () => {
    setError('')
    setLoading(true)
    try {
      const p = await previewFromStore(storeURL.trim())
      if (!p) return
      setPreview(p)
      setOverrideName(p.name)
      setScope({ kind: 'instances', ids: [] })
      setStep('scope')
    } catch (e: any) {
      setError(e?.message || '下载或解析失败')
    } finally {
      setLoading(false)
    }
  }

  // If the user closes the modal without committing, discard the staged preview.
  useEffect(() => {
    return () => {
      if (preview?.stagingToken) {
        cancelPreview(preview.stagingToken).catch(() => {})
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const pickLocal = async () => {
    setError('')
    setLoading(true)
    try {
      const p = await previewFromLocal('')
      if (!p) return
      setPreview(p)
      setOverrideName(p.name)
      setScope({ kind: 'instances', ids: [] })
      setStep('scope')
    } catch (e: any) {
      setError(e?.message || '加载扩展失败')
    } finally {
      setLoading(false)
    }
  }

  const confirm = async () => {
    if (!preview) return
    setLoading(true)
    setError('')
    try {
      await commitExtension(preview.stagingToken, scope, overrideName, preview.duplicateOf)
      setPreview(null) // prevent unmount-cancel from firing on a committed token
      onCommitted()
    } catch (e: any) {
      setError(e?.message || '保存失败')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    if (preview?.stagingToken) cancelPreview(preview.stagingToken).catch(() => {})
    setPreview(null)
    setStep('choose')
    onClose()
  }

  return (
    <Modal open={open} onClose={handleClose} title="添加扩展" width="560px">
      <div className="p-4 space-y-4">
        {error && <div className="text-sm text-rose-500">{error}</div>}

        {step === 'choose' && (
          <div>
            <div className="flex border-b border-[var(--color-border-muted)] mb-3">
              <button className={tabBtn(activeTab === 'store')} onClick={() => setActiveTab('store')}>从扩展商店</button>
              <button className={tabBtn(activeTab === 'local')} onClick={() => setActiveTab('local')}>从本地文件</button>
            </div>
            {activeTab === 'store' ? (
              <div className="space-y-3">
                <input
                  value={storeURL}
                  onChange={(e) => setStoreURL(e.target.value)}
                  placeholder="粘贴 Chrome / Edge 商店详情页 URL"
                  className="h-9 w-full rounded-md border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-3 text-sm"
                />
                <Button onClick={pickStore} loading={loading} disabled={!storeURL.trim()}>下载并解析</Button>
              </div>
            ) : (
              <div className="space-y-3">
                <p className="text-sm text-[var(--color-text-secondary)]">选择 <code>.crx</code> 或 <code>.zip</code> 文件。</p>
                <Button onClick={pickLocal} loading={loading}>选择文件...</Button>
              </div>
            )}
          </div>
        )}

        {step === 'scope' && preview && (
          <div className="space-y-4">
            <div className="flex items-start gap-3">
              <img src={preview.iconDataURL || placeholder} alt="" className="w-12 h-12 rounded object-contain bg-[var(--color-bg-muted)]" />
              <div className="flex-1">
                <input
                  className="h-8 w-full rounded border border-[var(--color-border-default)] bg-[var(--color-bg-surface)] px-2 text-sm font-medium"
                  value={overrideName}
                  onChange={(e) => setOverrideName(e.target.value)}
                />
                <p className="text-xs text-[var(--color-text-muted)] mt-1">提供方：{preview.provider || '未知'} · v{preview.version || '—'}</p>
              </div>
            </div>
            {preview.duplicateOf && (
              <div className="rounded bg-amber-50 text-amber-700 px-3 py-2 text-xs">
                检测到已安装同名扩展，保存将覆盖（要求两侧 chrome_id 一致）。
              </div>
            )}
            <ScopeSelector value={scope} onChange={setScope} />
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={handleClose}>取消</Button>
              <Button onClick={confirm} loading={loading}>完成</Button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  )
}
