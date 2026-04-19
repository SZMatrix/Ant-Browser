export type ScopeKind = 'instances' | 'groups' | 'all'

export interface ExtensionScope {
  kind: ScopeKind
  ids: string[]
}

export type SourceType = 'store' | 'local_crx' | 'local_zip'
export type StoreVendor = '' | 'chrome' | 'edge'

export type InstallStatus = 'succeeded' | 'installing' | 'failed'

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
  staleScopeIds: string[]
  installStatus: InstallStatus
  installError: string
}

export interface ExtensionMetadata {
  chromeId: string
  name: string
  provider: string
  description: string
  version: string
  iconDataURL: string
  sourceType: SourceType
  storeVendor: StoreVendor
  sourceUrl: string
  duplicateOf: string
}
