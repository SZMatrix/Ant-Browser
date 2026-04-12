import { Button, FormItem, Input, Select } from '../../../shared/components'

interface DirectImportForm {
  proxyName: string
  protocol: 'http' | 'https' | 'socks5'
  server: string
  port: string
  username: string
  password: string
}

interface DirectTestSpeedResult {
  ok: boolean
  latencyMs: number
  error: string
}

interface DirectHealthResult {
  ok: boolean
  ip: string
  country: string
  fraudScore: number
  isResidential: boolean
  error: string
}

const DIRECT_PROXY_PROTOCOL_OPTIONS = [
  { value: 'http', label: 'HTTP' },
  { value: 'https', label: 'HTTPS' },
  { value: 'socks5', label: 'SOCKS5' },
] as const

interface DirectProxyEditorProps {
  form: DirectImportForm
  onFormChange: (updater: (prev: DirectImportForm) => DirectImportForm) => void
  testSpeedResult: DirectTestSpeedResult | null
  testSpeedLoading: boolean
  healthResult: DirectHealthResult | null
  healthLoading: boolean
  onTestSpeed: () => void
  onHealthCheck: () => void
  showProxyName?: boolean
}

export function DirectProxyEditor({
  form,
  onFormChange,
  testSpeedResult,
  testSpeedLoading,
  healthResult,
  healthLoading,
  onTestSpeed,
  onHealthCheck,
  showProxyName,
}: DirectProxyEditorProps) {
  const showName = showProxyName ?? true
  return (
    <>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <FormItem label="代理协议" required>
          <Select
            options={[...DIRECT_PROXY_PROTOCOL_OPTIONS]}
            value={form.protocol}
            onChange={e => onFormChange(prev => ({ ...prev, protocol: e.target.value as DirectImportForm['protocol'] }))}
          />
        </FormItem>
        {showName && (
          <FormItem label="代理名称（可选）">
            <Input
              value={form.proxyName}
              onChange={e => onFormChange(prev => ({ ...prev, proxyName: e.target.value }))}
              placeholder="例如：香港节点"
            />
          </FormItem>
        )}
        <FormItem label="代理地址" required>
          <Input
            value={form.server}
            onChange={e => onFormChange(prev => ({ ...prev, server: e.target.value }))}
            placeholder="例如：127.0.0.1 或 hk.example.com"
          />
        </FormItem>
        <FormItem label="代理端口" required>
          <Input
            type="number"
            min={1}
            max={65535}
            value={form.port}
            onChange={e => onFormChange(prev => ({ ...prev, port: e.target.value }))}
            placeholder="例如：1080"
          />
        </FormItem>
        <FormItem label="账号（可选）">
          <Input
            value={form.username}
            onChange={e => onFormChange(prev => ({ ...prev, username: e.target.value }))}
            placeholder="留空则不使用认证"
          />
        </FormItem>
        <FormItem label="密码（可选）">
          <Input
            type="password"
            value={form.password}
            onChange={e => onFormChange(prev => ({ ...prev, password: e.target.value }))}
            placeholder="留空则不使用密码"
          />
        </FormItem>
      </div>
      <div className="space-y-2 mt-1">
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={onTestSpeed}
            loading={testSpeedLoading}
            disabled={!form.server.trim() || !form.port.trim()}
          >
            测速
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={onHealthCheck}
            loading={healthLoading}
            disabled={!form.server.trim() || !form.port.trim()}
          >
            IP 健康检测
          </Button>
        </div>
        {testSpeedResult && (
          <div className={`text-sm px-3 py-2 rounded ${testSpeedResult.ok ? 'bg-[var(--color-bg-secondary)] text-[var(--color-success)]' : 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400'}`}>
            {testSpeedResult.ok
              ? `连接成功，延迟：${testSpeedResult.latencyMs} ms`
              : `连接失败：${testSpeedResult.error}`}
          </div>
        )}
        {healthResult && (
          <div className={`text-sm px-3 py-2 rounded ${healthResult.ok ? 'bg-[var(--color-bg-secondary)]' : 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400'}`}>
            {healthResult.ok ? (
              <div className="space-y-0.5">
                <div>出口 IP：<span className="font-mono">{healthResult.ip}</span></div>
                <div>
                  国家/地区：{healthResult.country || '-'}
                  {' | '}欺诈分：<span className={healthResult.fraudScore > 60 ? 'text-red-500' : healthResult.fraudScore > 30 ? 'text-yellow-500' : 'text-[var(--color-success)]'}>{healthResult.fraudScore}</span>
                  {' | '}{healthResult.isResidential ? '住宅 IP' : '数据中心 IP'}
                </div>
              </div>
            ) : (
              <div>检测失败：{healthResult.error}</div>
            )}
          </div>
        )}
      </div>
    </>
  )
}

export type { DirectImportForm, DirectTestSpeedResult, DirectHealthResult }
