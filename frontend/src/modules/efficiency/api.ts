import type { ExtensionView, ExtensionPreview, ExtensionChangeResult, ExtensionScope } from './types'

const bindings = async (): Promise<any> => {
  try { return await import('../../wailsjs/go/main/App') } catch { return null }
}

export async function listExtensions(): Promise<ExtensionView[]> {
  const b = await bindings()
  if (b?.ExtensionList) return (await b.ExtensionList()) || []
  return []
}

export async function previewFromLocal(path = ''): Promise<ExtensionPreview | null> {
  const b = await bindings()
  if (!b?.ExtensionPreviewFromLocal) throw new Error('ExtensionPreviewFromLocal 未就绪')
  return await b.ExtensionPreviewFromLocal(path)
}

export async function previewFromStore(storeURL: string): Promise<ExtensionPreview | null> {
  const b = await bindings()
  if (!b?.ExtensionPreviewFromStore) throw new Error('ExtensionPreviewFromStore 未就绪')
  return await b.ExtensionPreviewFromStore(storeURL)
}

export async function commitExtension(
  stagingToken: string,
  scope: ExtensionScope,
  overrideName: string,
  duplicateOf: string,
): Promise<ExtensionView> {
  const b = await bindings()
  return await b.ExtensionCommit(stagingToken, scope, overrideName, duplicateOf)
}

export async function cancelPreview(stagingToken: string): Promise<void> {
  const b = await bindings()
  if (b?.ExtensionCancelPreview) await b.ExtensionCancelPreview(stagingToken)
}

export async function setEnabled(id: string, enabled: boolean): Promise<ExtensionChangeResult> {
  const b = await bindings()
  return await b.ExtensionSetEnabled(id, enabled)
}

export async function updateScope(id: string, scope: ExtensionScope): Promise<ExtensionChangeResult> {
  const b = await bindings()
  return await b.ExtensionUpdateScope(id, scope)
}

export async function renameExtension(id: string, name: string): Promise<void> {
  const b = await bindings()
  if (b?.ExtensionRename) await b.ExtensionRename(id, name)
}

export async function deleteExtension(id: string): Promise<ExtensionChangeResult> {
  const b = await bindings()
  return await b.ExtensionDelete(id)
}

export async function getPendingRestarts(): Promise<Record<string, string[]>> {
  const b = await bindings()
  if (b?.ExtensionGetPendingRestarts) return (await b.ExtensionGetPendingRestarts()) || {}
  return {}
}
