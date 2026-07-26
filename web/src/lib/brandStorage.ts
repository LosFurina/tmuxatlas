const currentPrefix = 'tmuxatlas:'
const legacyPrefix = 'guppi:'

export function getBrandStorage(key: string): string | null {
  const currentKey = currentPrefix + key
  const current = localStorage.getItem(currentKey)
  if (current !== null) return current

  const legacy = localStorage.getItem(legacyPrefix + key)
  if (legacy !== null) {
    localStorage.setItem(currentKey, legacy)
  }
  return legacy
}

export function setBrandStorage(key: string, value: string): void {
  localStorage.setItem(currentPrefix + key, value)
}
