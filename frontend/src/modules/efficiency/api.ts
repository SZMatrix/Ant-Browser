import type {
  ExtensionView,
  ExtensionMetadata,
  ExtensionScope,
} from './types'

const bindings = async (): Promise<any> => {
  try { return await import('../../wailsjs/go/main/App') } catch { return null }
}

export async function listExtensions(): Promise<ExtensionView[]> {
  const b = await bindings()
  if (b?.ExtensionList) return (await b.ExtensionList()) || []
  return []
}

export async function getExtension(id: string): Promise<ExtensionView | null> {
  const b = await bindings()
  if (!b?.ExtensionGet) return null
  try { return await b.ExtensionGet(id) } catch { return null }
}

export async function identifyFromLocal(path = ''): Promise<ExtensionMetadata | null> {
  const b = await bindings()
  if (!b?.ExtensionIdentifyFromLocal) throw new Error('ExtensionIdentifyFromLocal 未就绪')
  return await b.ExtensionIdentifyFromLocal(path)
}

export async function identifyFromStore(storeURL: string): Promise<ExtensionMetadata | null> {
  const b = await bindings()
  if (!b?.ExtensionIdentifyFromStore) throw new Error('ExtensionIdentifyFromStore 未就绪')
  return await b.ExtensionIdentifyFromStore(storeURL)
}

export async function createInstalling(
  meta: ExtensionMetadata,
  scope: ExtensionScope,
  overrideName: string,
): Promise<ExtensionView> {
  const b = await bindings()
  return await b.ExtensionCreateInstalling(meta, scope, overrideName)
}

export async function retryInstall(id: string): Promise<void> {
  const b = await bindings()
  if (b?.ExtensionRetryInstall) await b.ExtensionRetryInstall(id)
}

export async function setEnabled(id: string, enabled: boolean): Promise<ExtensionView> {
  const b = await bindings()
  return await b.ExtensionSetEnabled(id, enabled)
}

export async function updateScope(id: string, scope: ExtensionScope): Promise<ExtensionView> {
  const b = await bindings()
  return await b.ExtensionUpdateScope(id, scope)
}

export async function renameExtension(id: string, name: string): Promise<void> {
  const b = await bindings()
  if (b?.ExtensionRename) await b.ExtensionRename(id, name)
}

export async function deleteExtension(id: string): Promise<ExtensionView> {
  const b = await bindings()
  return await b.ExtensionDelete(id)
}
