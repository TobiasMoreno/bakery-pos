export function parseARSCents(value: string): number | undefined {
  const normalized = value.trim()
  if (!/^\d+(?:\.\d{1,2})?$/.test(normalized)) return undefined
  const parts = normalized.split('.')
  const whole = parts[0] ?? '0'
  const decimals = parts[1] ?? ''
  const cents = BigInt(whole) * 100n + BigInt(decimals.padEnd(2, '0'))
  if (cents <= 0n || cents > BigInt(Number.MAX_SAFE_INTEGER)) return undefined
  return Number(cents)
}

export function centsToInput(cents: number): string {
  const value = BigInt(cents)
  return `${value / 100n}.${String(value % 100n).padStart(2, '0')}`
}

export function formatARS(cents: number): string {
  const value = BigInt(cents)
  const whole = (value / 100n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  const decimals = (value % 100n).toString().padStart(2, '0')
  return `$ ${whole},${decimals}`
}
