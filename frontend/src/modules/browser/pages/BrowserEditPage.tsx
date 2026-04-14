import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { FolderOpen, Layers, Pencil } from 'lucide-react'
import { Button, Card, ConfirmModal, FormItem, Input, Modal, Select, Textarea, toast } from '../../../shared/components'
import type { BrowserCore, BrowserProfileInput, BrowserProxy, BrowserGroup } from '../types'
import { createBrowserProfile, fetchAllTags, fetchBrowserCores, fetchBrowserProfiles, fetchBrowserProxies, fetchBrowserSettings, fetchGroups, openUserDataDir, updateBrowserProfile, proxyTestSpeed, proxyCheckIPHealth } from '../api'
import { FingerprintPanel } from '../components/FingerprintPanel'
import { TagInput } from '../components/TagInput'
import { GroupSelector } from '../components/GroupSelector'
import { ProxyPickerModal } from '../components/ProxyPickerModal'
import { DirectProxyEditor } from '../components/DirectProxyEditor'
import type { DirectImportForm, DirectTestSpeedResult, DirectHealthResult } from '../components/DirectProxyEditor'
import { buildDirectProxyConfig, parseDirectProxyConfig } from '../utils/directProxy'

const fallbackLowLaunchArgs = ['--disable-sync', '--no-first-run']

function normalizeLaunchArgs(args: string[]): string[] {
  return (args || []).map(item => item.trim()).filter(Boolean)
}

function resolveDefaultLaunchArgs(args: string[]): string[] {
  const normalized = normalizeLaunchArgs(args)
  return normalized.length > 0 ? normalized : fallbackLowLaunchArgs
}

export function BrowserEditPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const isCreate = id === 'new'
  const [formData, setFormData] = useState<BrowserProfileInput>({
    profileName: '',
    userDataDir: '',
    coreId: '',
    fingerprintArgs: [],
    proxyId: '__direct__',
    proxyConfig: '',
    launchArgs: [],
    tags: [],
    keywords: [],
    groupId: '',
  })
  const [cores, setCores] = useState<BrowserCore[]>([])
  const [proxies, setProxies] = useState<BrowserProxy[]>([])
  const [groups, setGroups] = useState<BrowserGroup[]>([])
  const [launchArgsText, setLaunchArgsText] = useState('')
  const [allTags, setAllTags] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [proxyPickerOpen, setProxyPickerOpen] = useState(false)
  const [isDirty, setIsDirty] = useState(false)
  const [leaveConfirm, setLeaveConfirm] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [directForm, setDirectForm] = useState<DirectImportForm>({
    proxyName: '',
    protocol: 'http',
    server: '',
    port: '',
    username: '',
    password: '',
  })
  const [directTestSpeedResult, setDirectTestSpeedResult] = useState<DirectTestSpeedResult | null>(null)
  const [directTestSpeedLoading, setDirectTestSpeedLoading] = useState(false)
  const [directHealthResult, setDirectHealthResult] = useState<DirectHealthResult | null>(null)
  const [directHealthLoading, setDirectHealthLoading] = useState(false)

  useEffect(() => {
    const loadData = async () => {
      const [coreList, proxyList, tagList, groupList, settings] = await Promise.all([
        fetchBrowserCores(),
        fetchBrowserProxies(),
        fetchAllTags(),
        fetchGroups(),
        fetchBrowserSettings(),
      ])
      const resolvedDefaultLaunchArgs = resolveDefaultLaunchArgs(settings.defaultLaunchArgs || [])
      setCores(coreList)
      setProxies(proxyList)
      setAllTags(tagList)
      setGroups(groupList)

      if (isCreate) {
        setLaunchArgsText(resolvedDefaultLaunchArgs.join('\n'))
        return
      }
      const list = await fetchBrowserProfiles()
      const current = list.find(item => item.profileId === id)
      if (!current) return
      const currentLaunchArgs = normalizeLaunchArgs(current.launchArgs)
      const normalizedCoreId = !current.coreId || current.coreId.toLowerCase() === 'default'
        ? ''
        : current.coreId
      setFormData({
        profileName: current.profileName,
        userDataDir: current.userDataDir,
        coreId: normalizedCoreId,
        fingerprintArgs: current.fingerprintArgs,
        proxyId: current.proxyId,
        proxyConfig: current.proxyConfig,
        launchArgs: currentLaunchArgs,
        tags: current.tags,
        keywords: current.keywords || [],
        groupId: current.groupId || '',
      })
      setLaunchArgsText(currentLaunchArgs.join('\n'))
      // Detect direct mode: proxyId is __direct__, restore saved custom config if any
      if (current.proxyId === '__direct__') {
        if (current.proxyConfig) {
          const parsed = parseDirectProxyConfig(current.proxyConfig)
          if (parsed.ok) {
            setDirectForm(prev => ({
              ...prev,
              protocol: parsed.form.protocol,
              server: parsed.form.server,
              port: parsed.form.port,
              username: parsed.form.username,
              password: parsed.form.password,
            }))
          }
        }
        setFormData(prev => ({ ...prev, proxyId: '__direct__' }))
      // Detect custom proxy: proxyId empty but proxyConfig present (backward compat)
      } else if (!current.proxyId && current.proxyConfig) {
        const parsed = parseDirectProxyConfig(current.proxyConfig)
        if (parsed.ok) {
          setDirectForm(prev => ({
            ...prev,
            protocol: parsed.form.protocol,
            server: parsed.form.server,
            port: parsed.form.port,
            username: parsed.form.username,
            password: parsed.form.password,
          }))
        }
        setFormData(prev => ({ ...prev, proxyId: '__custom__' }))
      // No proxyId and no proxyConfig: treat as direct
      } else if (!current.proxyId) {
        setFormData(prev => ({ ...prev, proxyId: '__direct__' }))
      }
    }
    loadData()
  }, [id, isCreate])

  const isPoolProxy = !!(formData.proxyId && formData.proxyId !== '__direct__' && formData.proxyId !== '__custom__')

  useEffect(() => {
    if (isPoolProxy) {
      const proxy = proxies.find(p => p.proxyId === formData.proxyId)
      if (proxy?.proxyConfig) {
        const parsed = parseDirectProxyConfig(proxy.proxyConfig)
        if (parsed.ok) {
          setDirectForm(prev => ({
            ...prev,
            protocol: parsed.form.protocol,
            server: parsed.form.server,
            port: parsed.form.port,
            username: parsed.form.username,
            password: parsed.form.password,
          }))
        }
      }
    }
  }, [formData.proxyId, proxies])

  const handleChange = (field: keyof BrowserProfileInput, value: string | string[]) => {
    setIsDirty(true)
    setFormData(prev => ({ ...prev, [field]: value }))
  }

  const handleSave = async () => {
    setSaving(true)
    let proxyConfig = formData.proxyConfig
    let proxyId = formData.proxyId
    if (formData.proxyId === '__custom__') {
      try {
        proxyConfig = buildDirectProxyConfig({
          protocol: directForm.protocol,
          server: directForm.server,
          port: directForm.port,
          username: directForm.username,
          password: directForm.password,
        })
      } catch (e: any) {
        setSaveError(e?.message || '自定义代理配置无效')
        setSaving(false)
        return
      }
      proxyId = ''
    } else if (formData.proxyId === '__direct__') {
      proxyId = '__direct__'
      // Preserve custom proxy config so switching back to custom retains settings
      if (directForm.server && directForm.port) {
        try {
          proxyConfig = buildDirectProxyConfig({
            protocol: directForm.protocol,
            server: directForm.server,
            port: directForm.port,
            username: directForm.username,
            password: directForm.password,
          })
        } catch {
          proxyConfig = ''
        }
      } else {
        proxyConfig = ''
      }
    } else if (proxyId) {
      proxyConfig = ''
    } else {
      proxyConfig = ''
    }
    const payload: BrowserProfileInput = {
      ...formData,
      proxyId,
      proxyConfig,
      launchArgs: normalizeLaunchArgs(launchArgsText.split('\n')),
    }
    try {
      if (isCreate) {
        await createBrowserProfile(payload)
        toast.success('配置已创建')
      } else if (id) {
        await updateBrowserProfile(id, payload)
        toast.success('配置已更新')
      }
      setIsDirty(false)
      navigate('/browser/list')
    } catch (error: any) {
      setSaveError(typeof error === 'string' ? error : error?.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleBack = () => {
    if (isDirty) { setLeaveConfirm(true) } else { navigate('/browser/list') }
  }

  const defaultCore = cores.find(c => c.isDefault)

  const handleOpenUserDataDir = async () => {
    if (!formData.userDataDir.trim()) {
      toast.error('请先输入用户数据目录')
      return
    }
    try {
      await openUserDataDir(formData.userDataDir)
    } catch (error: unknown) {
      toast.error((error as Error)?.message || '打开目录失败')
    }
  }

  const handleDirectTestSpeed = async () => {
    let proxyConfig: string
    try {
      proxyConfig = buildDirectProxyConfig({
        protocol: directForm.protocol,
        server: directForm.server,
        port: directForm.port,
        username: directForm.username,
        password: directForm.password,
      })
    } catch (e: any) {
      toast.error(e?.message || '代理配置无效')
      return
    }
    setDirectTestSpeedLoading(true)
    try {
      const result = await proxyTestSpeed(proxyConfig)
      setDirectTestSpeedResult(result)
    } catch (e: any) {
      setDirectTestSpeedResult({ ok: false, latencyMs: 0, error: e?.message || '测速失败' })
    } finally {
      setDirectTestSpeedLoading(false)
    }
  }

  const handleDirectHealthCheck = async () => {
    let proxyConfig: string
    try {
      proxyConfig = buildDirectProxyConfig({
        protocol: directForm.protocol,
        server: directForm.server,
        port: directForm.port,
        username: directForm.username,
        password: directForm.password,
      })
    } catch (e: any) {
      toast.error(e?.message || '代理配置无效')
      return
    }
    setDirectHealthLoading(true)
    try {
      const raw = await proxyCheckIPHealth(proxyConfig)
      setDirectHealthResult({ ok: raw.ok, ip: raw.ip, country: raw.country, fraudScore: raw.fraudScore, isResidential: raw.isResidential, error: raw.error })
    } catch (e: any) {
      setDirectHealthResult({ ok: false, ip: '', country: '', fraudScore: 0, isResidential: false, error: e?.message || '检测失败' })
    } finally {
      setDirectHealthLoading(false)
    }
  }

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="sticky top-0 z-10 bg-[var(--color-bg-base)] -mx-5 px-5 py-3">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold text-[var(--color-text-primary)]">{isCreate ? '新建配置' : '编辑配置'}</h1>
            <p className="text-sm text-[var(--color-text-muted)] mt-1">完善指纹与启动参数</p>
          </div>
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={handleBack}>返回列表</Button>
            <Button size="sm" onClick={handleSave} loading={saving}>保存配置</Button>
          </div>
        </div>
      </div>

      <Card title="基础信息" subtitle="实例与配置名称">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <FormItem label="配置名称" required>
            <Input value={formData.profileName} onChange={e => handleChange('profileName', e.target.value)} placeholder="请输入配置名称" />
          </FormItem>
          <FormItem label="用户数据目录（留空自动生成）">
            <div className="flex gap-2">
              <Input
                value={formData.userDataDir}
                onChange={e => handleChange('userDataDir', e.target.value)}
                placeholder="留空自动生成"
                className="flex-1"
              />
              <Button variant="secondary" size="sm" onClick={handleOpenUserDataDir} title="在资源管理器中打开">
                <FolderOpen className="w-4 h-4" />
              </Button>
            </div>
          </FormItem>
          <FormItem label="内核">
            <Select
              value={formData.coreId}
              onChange={e => handleChange('coreId', e.target.value)}
              options={
                cores.length > 0 ? [
                  { value: '', label: defaultCore ? `使用默认 (${defaultCore.coreName})` : '使用默认内核' },
                  ...cores.map(c => ({ value: c.coreId, label: c.coreName })),
                ] : [
                  { value: '', label: '暂无内核，请添加内核' }
                ]
              }
            />
          </FormItem>
          <FormItem label="标签">
            <TagInput
              value={formData.tags}
              onChange={tags => handleChange('tags', tags)}
              suggestions={allTags}
              placeholder="输入标签后按回车，支持从已有标签选择"
            />
          </FormItem>
          <FormItem label="分组">
            <GroupSelector
              groups={groups}
              value={formData.groupId || ''}
              onChange={groupId => handleChange('groupId', groupId)}
              placeholder="未分组"
              className="w-full"
            />
          </FormItem>
        </div>
      </Card>

      <Card title="代理配置" subtitle="选择代理池中的代理或自定义配置">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <FormItem label="代理池选择">
            <div className="flex gap-2">
              <Select
                value={formData.proxyId}
                onChange={e => handleChange('proxyId', e.target.value)}
                options={[
                  { value: '__direct__', label: '直连' },
                  { value: '__custom__', label: '自定义' },
                  { divider: true as const },
                  ...proxies.map(p => ({ value: p.proxyId, label: p.proxyName || p.proxyId })),
                ]}
                className="flex-1"
              />
              {formData.proxyId !== '__custom__' && (
                <Button variant="secondary" size="sm" onClick={() => setProxyPickerOpen(true)} title="按分组选择代理">
                  <Layers className="w-4 h-4" />
                </Button>
              )}
              {isPoolProxy && (
                <Button variant="secondary" size="sm" onClick={() => handleChange('proxyId', '__custom__')} title="转为自定义编辑">
                  <Pencil className="w-4 h-4" />
                </Button>
              )}
            </div>
          </FormItem>
        </div>
        {(formData.proxyId === '__custom__' || isPoolProxy) && (
          <>
            <hr className="my-4 border-[var(--color-border)]" />
            <DirectProxyEditor
              form={directForm}
              onFormChange={setDirectForm}
              testSpeedResult={directTestSpeedResult}
              testSpeedLoading={directTestSpeedLoading}
              healthResult={directHealthResult}
              healthLoading={directHealthLoading}
              onTestSpeed={handleDirectTestSpeed}
              onHealthCheck={handleDirectHealthCheck}
              showProxyName={false}
              readOnly={isPoolProxy}
            />
          </>
        )}
      </Card>

      <ProxyPickerModal
        open={proxyPickerOpen}
        currentProxyId={formData.proxyId}
        onSelect={proxy => handleChange('proxyId', proxy.proxyId)}
        onClose={() => setProxyPickerOpen(false)}
      />

      <Card title="指纹配置" subtitle="配置浏览器指纹参数">
        <FingerprintPanel
          value={formData.fingerprintArgs}
          onChange={args => handleChange('fingerprintArgs', args)}
        />
      </Card>

      <Card title="启动参数" subtitle={isCreate ? '新建时默认填入轻量参数模板，直接改这里即可' : '每行一个参数'}>
        <div className="space-y-2">
          <Textarea
            value={launchArgsText}
            onChange={e => { setLaunchArgsText(e.target.value); setIsDirty(true) }}
            rows={6}
            placeholder="--disable-sync"
          />
          {isCreate && (
            <p className="text-xs text-[var(--color-text-muted)]">这里默认就是轻量参数模板；需要更复杂的参数，直接在此基础上修改。</p>
          )}
        </div>
      </Card>

      <ConfirmModal
        open={leaveConfirm}
        onClose={() => setLeaveConfirm(false)}
        onConfirm={() => navigate('/browser/list')}
        title="放弃未保存的更改？"
        content="当前页面有未保存的修改，离开后将丢失这些更改。"
        confirmText="放弃并离开"
        cancelText="继续编辑"
        danger
      />

      <Modal
        open={!!saveError}
        onClose={() => setSaveError('')}
        title="保存失败"
        width="420px"
        footer={<Button onClick={() => setSaveError('')}>知道了</Button>}
      >
        <div className="text-[var(--color-text-secondary)]">{saveError}</div>
      </Modal>
    </div>
  )
}
