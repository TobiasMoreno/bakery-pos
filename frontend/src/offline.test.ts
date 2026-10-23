import 'fake-indexeddb/auto'
import { beforeAll, describe, expect, it } from 'vitest'
import type { PreparedContext } from './offline'
import { closePreparedShiftOffline, configureOfflineUnlock, getOfflineUnlockUser, initializeOfflineStorage, loadPreparedContext, openPreparedShiftOffline, pendingOperations, savePreparedContext, stableStringify, verifyOfflineUnlock } from './offline'

const context: PreparedContext = {
  device: { id: '10000000-0000-4000-8000-000000000001', device_id: '10000000-0000-4000-8000-000000000002', device_name: 'Test', user_id: '10000000-0000-4000-8000-000000000003', status: 'active', fencing_version: 4, acquired_at: '2026-10-12T12:00:00Z' },
  shift: { id: '10000000-0000-4000-8000-000000000004', status: 'open', version: 7, device_session_id: '10000000-0000-4000-8000-000000000001', opened_by: '10000000-0000-4000-8000-000000000003', opening_cash_cents: 10000, opened_at: '2026-10-12T12:00:00Z', expected_cash_cents: 13000 },
  snapshot: { version: 9, generated_at: '2026-10-12T12:00:00Z', products: [] },
  authorization: { token: 'signed-token', user_id: '10000000-0000-4000-8000-000000000003', device_id: '10000000-0000-4000-8000-000000000002', device_session_id: '10000000-0000-4000-8000-000000000001', fencing_version: 4, cash_shift_id: '10000000-0000-4000-8000-000000000004', shift_version: 7, catalog_version: 9, issued_at: '2026-10-12T12:00:00Z', expires_at: '2099-10-13T12:00:00Z' },
}

beforeAll(async () => {
  await initializeOfflineStorage()
  await savePreparedContext(context)
})

describe('stableStringify', () => {
  it('canonicalizes nested objects without changing array order', () => {
    expect(stableStringify({ z: 1, a: { y: 2, b: [3, { d: 4, c: 5 }] } })).toBe(
      '{"a":{"b":[3,{"c":5,"d":4}],"y":2},"z":1}',
    )
  })

  it('produces the same payload for different property insertion order', () => {
    expect(stableStringify({ b: 2, a: 1 })).toBe(stableStringify({ a: 1, b: 2 }))
  })
})

describe('offline journal', () => {
  it('persists each shift transition and its prepared state atomically in strict order', async () => {
    const close = await closePreparedShiftOffline(context, 12900, 'Relevo')
    expect((await loadPreparedContext())?.shift).toMatchObject({
      id: context.shift.id,
      status: 'provisional_closed',
      version: context.shift.version + 1,
      counted_cash_cents: 12900,
      difference_cents: -100,
    })

    const open = await openPreparedShiftOffline(close.context, 12900, 'Recibido del turno anterior')
    expect(open.operation.sequence).toBe(close.operation.sequence + 1)
    expect(open.operation.previous_hash).toBe(close.operation.payload_hash)
    expect((await loadPreparedContext())?.shift).toEqual(open.context.shift)
    expect((await pendingOperations()).map((operation) => operation.operation_type)).toEqual(['shift_close', 'shift_open'])

    await expect(openPreparedShiftOffline(close.context, 12900, '')).rejects.toThrow('El turno local cambió')
    expect((await pendingOperations()).map((operation) => operation.operation_id)).toEqual([
      close.operation.operation_id,
      open.operation.operation_id,
    ])
    expect((await loadPreparedContext())?.shift).toEqual(open.context.shift)
  })
})

describe('offline unlock', () => {
  it('stores only a derived verifier and unlocks the cached authorized user', async () => {
    const user = { id: context.authorization.user_id, name: 'Dueño', username: 'owner', role: 'owner' as const, permissions: ['sales:write'] }
    await configureOfflineUnlock(user, 'correct-horse')
    expect(await verifyOfflineUnlock('wrong-horse!')).toBeNull()
    expect(await verifyOfflineUnlock('correct-horse')).toEqual(user)
    expect(await getOfflineUnlockUser()).toEqual(user)
  })
})
