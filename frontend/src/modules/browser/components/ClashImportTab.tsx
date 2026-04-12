import { Button, FormItem, Input, Textarea } from '../../../shared/components'

interface ClashImportTabProps {
  importUrl: string
  importResolvedUrl: string
  importText: string
  fetchingImportUrl: boolean
  onUrlChange: (value: string) => void
  onTextChange: (value: string) => void
  onFetchUrl: () => void
}

export function ClashImportTab({
  importUrl,
  importResolvedUrl,
  importText,
  fetchingImportUrl,
  onUrlChange,
  onTextChange,
  onFetchUrl,
}: ClashImportTabProps) {
  return (
    <>
      <FormItem label="订阅 URL（可选）">
        <div className="flex gap-2">
          <Input
            value={importUrl}
            onChange={e => onUrlChange(e.target.value)}
            placeholder="https://example.com/clash/subscription"
            className="flex-1"
          />
          <Button
            variant="secondary"
            onClick={onFetchUrl}
            loading={fetchingImportUrl}
            disabled={!importUrl.trim()}
          >
            从 URL 获取
          </Button>
        </div>
        {importResolvedUrl.trim() && (
          <p className="text-xs text-[var(--color-success)] mt-1 break-all">
            已绑定订阅：{importResolvedUrl}
          </p>
        )}
        <p className="text-xs text-[var(--color-text-muted)] mt-1">获取成功后会自动回填 YAML 文本，并尝试自动填充 DNS 与建议分组；自动刷新时间请在列表顶部统一配置</p>
      </FormItem>
      <Textarea
        value={importText}
        onChange={e => onTextChange(e.target.value)}
        rows={12}
        placeholder={`proxies:\n  - name: vless-v6\n    type: vless\n    server: example.com\n    port: 443\n    uuid: your-uuid\n    ...`}
      />
    </>
  )
}
