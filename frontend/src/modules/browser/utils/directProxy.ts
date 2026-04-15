// Direct proxy form helpers shared by proxy import and instance edit pages.
// The form lets users fill in proxy fields directly without saving to the proxy pool.

export type DirectProxyProtocol = 'http' | 'https' | 'socks5'

export interface DirectProxyForm {
  protocol: DirectProxyProtocol
  server: string
  port: string
  username: string
  password: string
}

export const DIRECT_PROXY_PROTOCOL_OPTIONS: { value: DirectProxyProtocol; label: string }[] = [
  { value: 'http', label: 'HTTP' },
  { value: 'https', label: 'HTTPS' },
  { value: 'socks5', label: 'SOCKS5' },
]

export const INITIAL_DIRECT_PROXY_FORM: DirectProxyForm = {
  protocol: 'http',
  server: '',
  port: '',
  username: '',
  password: '',
}

function formatDirectProxyHost(raw: string): string {
  const host = raw.trim()
  if (!host) return ''
  if (host.startsWith('[') && host.endsWith(']')) {
    return host
  }
  return host.includes(':') ? `[${host}]` : host
}

function normalizeDirectProxyConfig(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  if (/^socket:\/\//i.test(trimmed)) {
    return trimmed.replace(/^socket:\/\//i, 'socks5://')
  }
  if (/^socks:\/\//i.test(trimmed)) {
    return trimmed.replace(/^socks:\/\//i, 'socks5://')
  }
  return trimmed
}

function containsPlaceholder(s: string): boolean {
  return /\{[a-zA-Z]+\}/.test(s)
}

/**
 * Build a normalized proxy URL string from direct form fields.
 * Throws an Error with a user-friendly Chinese message if validation fails.
 * Supports {profileName} placeholders — when detected, skips strict URL validation.
 */
export function buildDirectProxyConfig(form: DirectProxyForm): string {
  const serverInput = form.server.trim()
  if (!serverInput) {
    throw new Error('请输入代理地址')
  }

  const portInput = form.port.trim()
  if (!portInput) {
    throw new Error('请输入代理端口')
  }

  const username = form.username.trim()
  const password = form.password

  const hasPlaceholder = [serverInput, portInput, username, password].some(containsPlaceholder)

  if (hasPlaceholder) {
    // When placeholders are present, skip strict validation and build raw config.
    // Placeholders will be resolved at copy/create time by the backend.
    const auth = username
      ? `${encodeURIComponent(username)}${password ? `:${encodeURIComponent(password)}` : ''}@`
      : ''
    return `${form.protocol}://${auth}${formatDirectProxyHost(serverInput)}:${portInput}`
  }

  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(serverInput)) {
    throw new Error('代理地址只需要填写主机名或 IP，不需要协议头')
  }

  if (!/^\d+$/.test(portInput)) {
    throw new Error('代理端口必须为数字')
  }

  const port = Number(portInput)
  if (port < 1 || port > 65535) {
    throw new Error('代理端口必须在 1-65535 之间')
  }

  if (password && !username) {
    throw new Error('填写密码时请同时填写账号')
  }

  const auth = username
    ? `${encodeURIComponent(username)}${password ? `:${encodeURIComponent(password)}` : ''}@`
    : ''
  const rawConfig = `${form.protocol}://${auth}${formatDirectProxyHost(serverInput)}:${port}`

  let parsedURL: URL
  try {
    parsedURL = new URL(rawConfig)
  } catch {
    throw new Error('请输入有效的代理地址')
  }

  if (!parsedURL.hostname) {
    throw new Error('请输入有效的代理地址')
  }

  return normalizeDirectProxyConfig(parsedURL.toString()).replace(/\/$/, '')
}

/**
 * Best-effort parse of an existing proxyConfig URL back into the direct form fields.
 * Returns the initial form (unchanged) when the string is empty or unparseable,
 * and a second boolean indicating whether parsing succeeded.
 */
export function parseDirectProxyConfig(raw: string): { form: DirectProxyForm; ok: boolean } {
  const trimmed = (raw || '').trim()
  if (!trimmed) {
    return { form: { ...INITIAL_DIRECT_PROXY_FORM }, ok: true }
  }

  const normalized = normalizeDirectProxyConfig(trimmed)
  let parsed: URL
  try {
    parsed = new URL(normalized)
  } catch {
    return { form: { ...INITIAL_DIRECT_PROXY_FORM }, ok: false }
  }

  const scheme = parsed.protocol.replace(/:$/, '').toLowerCase()
  if (scheme !== 'http' && scheme !== 'https' && scheme !== 'socks5') {
    return { form: { ...INITIAL_DIRECT_PROXY_FORM }, ok: false }
  }

  const hostname = parsed.hostname.replace(/^\[(.*)\]$/, '$1')
  const port = parsed.port || ''

  let username = ''
  let password = ''
  try {
    username = parsed.username ? decodeURIComponent(parsed.username) : ''
    password = parsed.password ? decodeURIComponent(parsed.password) : ''
  } catch {
    username = parsed.username || ''
    password = parsed.password || ''
  }

  return {
    form: {
      protocol: scheme as DirectProxyProtocol,
      server: hostname,
      port,
      username,
      password,
    },
    ok: true,
  }
}
