import { describe, expect, it } from 'vitest'
import { calculateLineSubtotal, centsToInput, formatARS, parseARSCents } from './money'

describe('money helpers', () => {
  it('parses ARS without floating point arithmetic', () => {
    expect(parseARSCents('9500.25')).toBe(950025)
    expect(parseARSCents('10')).toBe(1000)
  })

  it('rejects ambiguous or non-positive values', () => {
    expect(parseARSCents('10.001')).toBeUndefined()
    expect(parseARSCents('0')).toBeUndefined()
    expect(parseARSCents('1e3')).toBeUndefined()
  })

  it('formats integer cents for display and forms', () => {
    expect(formatARS(950025)).toBe('$ 9.500,25')
    expect(centsToInput(950025)).toBe('9500.25')
  })
})

describe('calculateLineSubtotal', () => {
  it('calculates 350 g at ARS 9,500 per kg using integer arithmetic', () => {
    expect(calculateLineSubtotal(950000, 'weight', '350')).toBe(332500)
  })

  it('rejects fractional quantities for unit sales', () => {
    expect(calculateLineSubtotal(10000, 'unit', '1.5')).toBeUndefined()
  })
})
