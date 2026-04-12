import { useState, type ReactNode } from 'react'
import { Button, FormItem, Input, Modal, Select, toast } from '../../../shared/components'
import { proxyCheckIPHealth, proxyTestSpeed } from '../api'
import type { ProxyIPHealthResult } from '../types'
import {
  DIRECT_PROXY_PROTOCOL_OPTIONS,
  buildDirectProxyConfig,
  type DirectProxyForm,
} from '../utils/directProxy'

export interface DirectProxyEditorProps {
  /** 表单当前值 */
  value: DirectProxyForm
  /** 表单值变化回调 */
  onChange: (next: DirectProxyForm) => void

  /** 可选的"代理名称"字段值。只有同时提供 onNameChange 时才会渲染。 */
  nameValue?: string
  onNameChange?: (value: string) => void
  nameLabel?: string
  namePlaceholder?: string

  /**
   * 原始字符串降级模式：当外部传入的 proxyConfig 无法解析到表单字段时使用，
   * 用户可以直接编辑原始代理字符串。
   */
  rawMode?: boolean
  rawValue?: string
  onRawChange?: (value: string) => void
  /** "切换为表单" 按钮点击回调；若不传则不展示该按钮。 */
  onSwitchToForm?: () => void

  /** 是否显示"检测"按钮。默认 true。 */
  showTestButton?: boolean

  /** 容器额外 className */
  className?: string
  /** 顶部右侧额外 slot */
  headerExtra?: ReactNode
}

/**
 * 统一的"直连代理"表单编辑器：
 *   - 在代理池"直接导入"和"新增/编辑实例"两处复用同一套字段和校验；
 *   - 内置基于代理模型的测速 + IP 健康检测，结果仅在当前组件内展示，不会写回代理池；
 *   - 支持原始字符串降级模式，避免解析失败时丢失用户已有的 proxyConfig。
 */
export function DirectProxyEditor(props: DirectProxyEditorProps) {
  const {
    value,
    onChange,
    nameValue,
    onNameChange,
    nameLabel = '代理名称（可选）',
    namePlaceholder = '例如：香港节点',
    rawMode = false,
    rawValue = '',
    onRawChange,
    onSwitchToForm,
    showTestButton = true,
    className,
    headerExtra,
  } = props

  const [testing, setTesting] = useState(false)
  const [speedResult, setSpeedResult] = useState<{ ok: boolean; latencyMs: number; error: string } | null>(null)
  const [healthResult, setHealthResult] = useState<ProxyIPHealthResult | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const clearResults = () => {
    setSpeedResult(null)
    setHealthResult(null)
  }

  function update<K extends keyof DirectProxyForm>(field: K, next: DirectProxyForm[K]) {
    onChange({ ...value, [field]: next })
    clearResults()
  }

  const handleRawInput = (next: string) => {
    onRawChange?.(next)
    clearResults()
  }

  const handleTest = async () => {
    let proxyConfig = ''
    try {
      if (rawMode) {
        proxyConfig = (rawValue || '').trim()
        if (!proxyConfig) {
          toast.error('请先输入代理地址')
          return
        }
      } else {
        proxyConfig = buildDirectProxyConfig(value)
      }
    } catch (error: any) {
      toast.error(error?.message || '代理配置无效')
      return
    }

    setTesting(true)
    clearResults()
    try {
      const [speed, health] = await Promise.all([
        proxyTestSpeed(proxyConfig),
        proxyCheckIPHealth(proxyConfig),
      ])
      setSpeedResult({ ok: speed.ok, latencyMs: speed.latencyMs, error: speed.error })
      setHealthResult(health)
      if (speed.ok) {
        toast.success(`测速成功：${speed.latencyMs} ms`)
      } else {
        toast.error(`测速失败：${speed.error || '未知错误'}`)
      }
    } catch (error: any) {
      toast.error(error?.message || '检测失败')
    } finally {
      setTesting(false)
    }
  }

  const showHeader = showTestButton || !!headerExtra
  const hasAnyResult = !!speedResult || !!healthResult

  return (
    <div className={className}>
      {showHeader && (
        <div className="flex items-center justify-end gap-2 mb-3">
          {headerExtra}
          {showTestButton && (
            <Button variant="secondary" size="sm" onClick={handleTest} loading={testing}>
              检测
            </Button>
          )}
        </div>
      )}

      {rawMode ? (
        <FormItem label="代理地址">
          <div className="flex gap-2">
            <Input
              value={rawValue}
              onChange={e => handleRawInput(e.target.value)}
              placeholder="http://127.0.0.1:7890"
              className="flex-1"
            />
            {onSwitchToForm && (
              <Button variant="secondary" size="sm" onClick={onSwitchToForm} title="切换为表单填写">
                切换为表单
              </Button>
            )}
          </div>
          <p className="text-xs text-[var(--color-text-muted)] mt-1">
            当前代理字符串无法自动解析为表单字段，保留原始文本以防丢失。
          </p>
        </FormItem>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormItem label="代理协议" required>
            <Select
              options={[...DIRECT_PROXY_PROTOCOL_OPTIONS]}
              value={value.protocol}
              onChange={e => update('protocol', e.target.value as DirectProxyForm['protocol'])}
            />
          </FormItem>
          {onNameChange && (
            <FormItem label={nameLabel}>
              <Input
                value={nameValue || ''}
                onChange={e => { onNameChange(e.target.value); clearResults() }}
                placeholder={namePlaceholder}
              />
            </FormItem>
          )}
          <FormItem label="代理地址" required>
            <Input
              value={value.server}
              onChange={e => update('server', e.target.value)}
              placeholder="例如：127.0.0.1 或 hk.example.com"
            />
          </FormItem>
          <FormItem label="代理端口" required>
            <Input
              type="number"
              min={1}
              max={65535}
              value={value.port}
              onChange={e => update('port', e.target.value)}
              placeholder="例如：1080"
            />
          </FormItem>
          <FormItem label="账号（可选）">
            <Input
              value={value.username}
              onChange={e => update('username', e.target.value)}
              placeholder="留空则不使用认证"
            />
          </FormItem>
          <FormItem label="密码（可选）">
            <Input
              type="password"
              value={value.password}
              onChange={e => update('password', e.target.value)}
              placeholder="留空则不使用密码"
            />
          </FormItem>
        </div>
      )}

      {hasAnyResult && (
        <div className="mt-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-secondary)] p-3 space-y-2">
          {speedResult && (
            <div className="flex items-center gap-2 text-sm">
              <span className="text-[var(--color-text-muted)]">测速：</span>
              {speedResult.ok ? (
                <span className="text-green-500">{speedResult.latencyMs} ms</span>
              ) : (
                <span className="text-red-500">{speedResult.error || '失败'}</span>
              )}
            </div>
          )}
          {healthResult && (
            <div className="text-sm">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-[var(--color-text-muted)]">IP 健康：</span>
                {healthResult.ok ? (
                  <>
                    <span className="text-[var(--color-text-primary)]">{healthResult.ip || '-'}</span>
                    {healthResult.country && (
                      <span className="text-[var(--color-text-secondary)]">
                        {healthResult.country}
                        {healthResult.region ? ` / ${healthResult.region}` : ''}
                        {healthResult.city ? ` / ${healthResult.city}` : ''}
                      </span>
                    )}
                    {healthResult.asOrganization && (
                      <span className="text-[var(--color-text-muted)]">{healthResult.asOrganization}</span>
                    )}
                    <span className="text-[var(--color-text-muted)]">欺诈分：{healthResult.fraudScore}</span>
                    {healthResult.isResidential && <span className="text-green-500">住宅</span>}
                  </>
                ) : (
                  <span className="text-red-500">{healthResult.error || '失败'}</span>
                )}
                <Button variant="secondary" size="sm" onClick={() => setDetailOpen(true)}>
                  查看详情
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      <Modal
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        title="IP 健康原始返回"
        width="760px"
        footer={<Button variant="secondary" onClick={() => setDetailOpen(false)}>关闭</Button>}
      >
        <div className="space-y-3">
          {healthResult && (
            <>
              <div className="text-xs text-[var(--color-text-muted)]">
                来源：{healthResult.source} | 时间：{healthResult.updatedAt}
              </div>
              {!healthResult.ok && (
                <div className="text-sm text-red-500">{healthResult.error || '检测失败'}</div>
              )}
              <pre className="max-h-[420px] overflow-auto text-xs leading-5 rounded-lg bg-[var(--color-bg-secondary)] border border-[var(--color-border)] p-3">
                {JSON.stringify(healthResult.rawData || {}, null, 2)}
              </pre>
            </>
          )}
        </div>
      </Modal>
    </div>
  )
}
