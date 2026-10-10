export function formatLoadValue(value: number | undefined): string {
  return (value ?? 0).toFixed(2)
}

export function formatMemoryMb(value: number | undefined): string {
  return (value ?? 0).toFixed(1)
}
