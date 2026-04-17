export type ScopeKind = 'instances' | 'groups'

export interface ExtensionScope {
  kind: ScopeKind
  ids: string[]
}

export type SourceType = 'store' | 'local_crx' | 'local_zip'
export type StoreVendor = '' | 'chrome' | 'edge'

export interface ExtensionView {
  extensionId: string
  chromeId: string
  name: string
  provider: string
  description: string
  version: string
  sourceType: SourceType
  storeVendor: StoreVendor
  sourceUrl: string
  enabled: boolean
  scope: ExtensionScope
  iconDataURL: string
  pendingRestartProfileIds: string[]
  staleScopeIds: string[]
}

export interface ExtensionPreview {
  stagingToken: string
  chromeId: string
  name: string
  provider: string
  description: string
  version: string
  sourceType: SourceType
  storeVendor: StoreVendor
  sourceUrl: string
  iconDataURL: string
  duplicateOf: string
}

export interface ExtensionChangeResult {
  extension: ExtensionView | null
  affectedProfileIds: string[]
  cdpSucceededIds: string[]
  pendingRestartIds: string[]
  notRunningIds: string[]
  cdpSupportedByKernel: boolean
}
