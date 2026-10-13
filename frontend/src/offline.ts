import type {
  CashShift,
  CatalogSnapshot,
  DeviceSession,
  OfflineAuthorization,
  OfflinePayload,
  OfflineOperation,
  SaleInput,
  SyncACK,
  User,
} from './api'

const databaseName = 'bakery-pos-offline'
const databaseVersion = 2
const unlockIterations = 310_000

export type CheckoutIntentStatus = 'ready_for_payment' | 'collecting' | 'consolidated' | 'cancelled'
export interface CheckoutIntent {
  id: string
  status: CheckoutIntentStatus
  total_cents: number
  cart: SaleInput['items']
  created_at: string
  operation_id?: string
}

export interface PreparedContext {
  device: DeviceSession
  shift: CashShift
  snapshot: CatalogSnapshot
  authorization: OfflineAuthorization
}

type JournalState = { key: 'journal'; sequence: number; last_hash: string }
type StoredOperation = OfflineOperation & { local_status: 'pending' | 'sending' | 'acknowledged' | 'conflict' | 'failed'; ack?: SyncACK }
type OfflineUnlockRecord = { key: 'active'; user: User; salt: string; verifier: string; iterations: number; configured_at: string }

export async function initializeOfflineStorage(): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  const db = await openDatabase()
  const tx = db.transaction('meta', 'readwrite')
  tx.objectStore('meta').put({ key: 'probe', value: crypto.randomUUID() })
  await transactionDone(tx)
  if (navigator.storage?.persist) await navigator.storage.persist().catch(() => false)
}

export async function savePreparedContext(context: PreparedContext): Promise<void> {
  const db = await openDatabase()
  const tx = db.transaction(['meta', 'catalog_snapshots', 'device_session', 'offline_authorization'], 'readwrite')
  tx.objectStore('catalog_snapshots').put(context.snapshot)
  tx.objectStore('device_session').put({ key: 'active', value: context.device })
  tx.objectStore('offline_authorization').put({ key: 'active', value: context.authorization })
  tx.objectStore('meta').put({ key: 'prepared', value: { shift: context.shift, catalog_version: context.snapshot.version } })
  const journal = await requestResult<JournalState | undefined>(tx.objectStore('meta').get('journal'))
  if (!journal) tx.objectStore('meta').put({ key: 'journal', sequence: 0, last_hash: '' } satisfies JournalState)
  await transactionDone(tx)
}

export async function loadPreparedContext(): Promise<PreparedContext | null> {
  const db = await openDatabase()
  const tx = db.transaction(['meta', 'catalog_snapshots', 'device_session', 'offline_authorization'])
  const prepared = await requestResult<{ key: string; value: { shift: CashShift; catalog_version: number } } | undefined>(tx.objectStore('meta').get('prepared'))
  const device = await requestResult<{ key: string; value: DeviceSession } | undefined>(tx.objectStore('device_session').get('active'))
  const authorization = await requestResult<{ key: string; value: OfflineAuthorization } | undefined>(tx.objectStore('offline_authorization').get('active'))
  const snapshot = prepared ? await requestResult<CatalogSnapshot | undefined>(tx.objectStore('catalog_snapshots').get(prepared.value.catalog_version)) : undefined
  await transactionDone(tx)
  if (!prepared || !device || !authorization || !snapshot) return null
  return { device: device.value, shift: prepared.value.shift, snapshot, authorization: authorization.value }
}

export async function createCheckoutIntent(cart: SaleInput['items'], totalCents: number): Promise<CheckoutIntent> {
  const intent: CheckoutIntent = { id: crypto.randomUUID(), status: 'ready_for_payment', total_cents: totalCents, cart: structuredClone(cart), created_at: new Date().toISOString() }
  const db = await openDatabase()
  const tx = db.transaction('checkout_intents', 'readwrite')
  tx.objectStore('checkout_intents').add(intent)
  await transactionDone(tx)
  return intent
}

export async function markIntentCollecting(id: string): Promise<CheckoutIntent> {
  const db = await openDatabase()
  const tx = db.transaction('checkout_intents', 'readwrite')
  const store = tx.objectStore('checkout_intents')
  const intent = await requestResult<CheckoutIntent | undefined>(store.get(id))
  if (!intent || intent.status !== 'ready_for_payment') throw new Error('El intento de cobro ya no está disponible.')
  intent.status = 'collecting'
  store.put(intent)
  await transactionDone(tx)
  return intent
}

export async function consolidateIntent(id: string, payload: SaleInput, context: PreparedContext): Promise<StoredOperation> {
  const payloadHash = await sha256(stableStringify(payload))
  const db = await openDatabase()
  const tx = db.transaction(['meta', 'checkout_intents', 'operations'], 'readwrite')
  const metaStore = tx.objectStore('meta')
  const intentStore = tx.objectStore('checkout_intents')
  const journal = await requestResult<JournalState | undefined>(metaStore.get('journal'))
  const intent = await requestResult<CheckoutIntent | undefined>(intentStore.get(id))
  if (!journal || !intent || intent.status !== 'collecting') throw new Error('No se pudo consolidar el cobro pendiente.')
  if (new Date(context.authorization.expires_at).getTime() < Date.now()) throw new Error('La autorización offline venció. Reconectate antes de iniciar otra venta.')
  const operation: StoredOperation = {
    operation_id: payload.operation_id,
    operation_type: 'sale',
    device_id: context.device.device_id,
    device_session_id: context.device.id,
    fencing_version: context.device.fencing_version,
    sequence: journal.sequence + 1,
    created_at_local: new Date().toISOString(),
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Argentina/Buenos_Aires',
    previous_hash: journal.last_hash,
    payload_hash: payloadHash,
    authorization: context.authorization.token,
    payload,
    local_status: 'pending',
  }
  tx.objectStore('operations').add(operation)
  intent.status = 'consolidated'
  intent.operation_id = operation.operation_id
  intentStore.put(intent)
  metaStore.put({ key: 'journal', sequence: operation.sequence, last_hash: operation.payload_hash } satisfies JournalState)
  await transactionDone(tx)
  return operation
}

export async function enqueueOfflineOperation(
  operationType: OfflineOperation['operation_type'],
  payload: OfflinePayload,
  context: PreparedContext,
  operationID = crypto.randomUUID(),
): Promise<StoredOperation> {
  if (new Date(context.authorization.expires_at).getTime() < Date.now()) throw new Error('La autorización offline venció. Reconectate antes de continuar.')
  const payloadHash = await sha256(stableStringify(payload))
  const db = await openDatabase()
  const tx = db.transaction(['meta', 'operations'], 'readwrite')
  const metaStore = tx.objectStore('meta')
  const journal = await requestResult<JournalState | undefined>(metaStore.get('journal'))
  if (!journal) throw new Error('El diario local no está preparado.')
  const operation: StoredOperation = {
    operation_id: operationID,
    operation_type: operationType,
    device_id: context.device.device_id,
    device_session_id: context.device.id,
    fencing_version: context.device.fencing_version,
    sequence: journal.sequence + 1,
    created_at_local: new Date().toISOString(),
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Argentina/Buenos_Aires',
    previous_hash: journal.last_hash,
    payload_hash: payloadHash,
    authorization: context.authorization.token,
    payload,
    local_status: 'pending',
  }
  tx.objectStore('operations').add(operation)
  metaStore.put({ key: 'journal', sequence: operation.sequence, last_hash: operation.payload_hash } satisfies JournalState)
  await transactionDone(tx)
  return operation
}

export async function configureOfflineUnlock(user: User, secret: string): Promise<void> {
  if (secret.length < 8) throw new Error('La clave local debe tener al menos 8 caracteres.')
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const verifier = await deriveUnlockVerifier(secret, salt, unlockIterations)
  const record: OfflineUnlockRecord = {
    key: 'active', user, salt: bytesToBase64(salt), verifier: bytesToBase64(verifier), iterations: unlockIterations, configured_at: new Date().toISOString(),
  }
  const db = await openDatabase()
  const tx = db.transaction('offline_unlock', 'readwrite')
  tx.objectStore('offline_unlock').put(record)
  await transactionDone(tx)
}

export async function getOfflineUnlockUser(): Promise<User | null> {
  const [record, context] = await Promise.all([getOfflineUnlockRecord(), loadPreparedContext()])
  if (!record || !context || new Date(context.authorization.expires_at).getTime() < Date.now()) return null
  return record.user
}

export async function verifyOfflineUnlock(secret: string): Promise<User | null> {
  const record = await getOfflineUnlockRecord()
  if (!record) return null
  const actual = await deriveUnlockVerifier(secret, base64ToBytes(record.salt), record.iterations)
  const expected = base64ToBytes(record.verifier)
  if (actual.length !== expected.length) return null
  let difference = 0
  for (let index = 0; index < actual.length; index += 1) difference |= actual[index]! ^ expected[index]!
  return difference === 0 ? record.user : null
}

export async function clearOfflineUnlock(): Promise<void> {
  const db = await openDatabase()
  const tx = db.transaction('offline_unlock', 'readwrite')
  tx.objectStore('offline_unlock').delete('active')
  await transactionDone(tx)
}

export async function cancelUncollectedIntent(id: string): Promise<void> {
  const db = await openDatabase()
  const tx = db.transaction('checkout_intents', 'readwrite')
  const store = tx.objectStore('checkout_intents')
  const intent = await requestResult<CheckoutIntent | undefined>(store.get(id))
  if (intent?.status === 'ready_for_payment') {
    intent.status = 'cancelled'
    store.put(intent)
  }
  await transactionDone(tx)
}

export async function blockingCollectingIntent(): Promise<CheckoutIntent | null> {
  const intents = await allFromStore<CheckoutIntent>('checkout_intents')
  return intents.find((intent) => intent.status === 'collecting') ?? null
}

export async function pendingOperations(): Promise<OfflineOperation[]> {
  const operations = await allFromStore<StoredOperation>('operations')
  return operations.filter((operation) => operation.local_status === 'pending' || operation.local_status === 'failed').sort((a, b) => a.sequence - b.sequence).map(stripLocalFields)
}

export async function pendingOperationCount(): Promise<number> {
  return (await pendingOperations()).length
}

export async function markSending(ids: string[]): Promise<void> {
  await updateOperations(ids, (operation) => ({ ...operation, local_status: 'sending' }))
}

export async function markSyncFailed(ids: string[]): Promise<void> {
  await updateOperations(ids, (operation) => ({ ...operation, local_status: 'failed' }))
}

export async function applySyncACKs(acks: SyncACK[]): Promise<void> {
  const byID = new Map(acks.map((ack) => [ack.operation_id, ack]))
  await updateOperations([...byID.keys()], (operation) => {
    const ack = byID.get(operation.operation_id)!
    return { ...operation, local_status: ack.status === 'acknowledged' ? 'acknowledged' : 'conflict', ack }
  })
}

export function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map((key) => `${JSON.stringify(key)}:${stableStringify(record[key])}`).join(',')}}`
}

async function sha256(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value)
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('')
}

function stripLocalFields(operation: StoredOperation): OfflineOperation {
  const envelope: Partial<StoredOperation> = { ...operation }
  delete envelope.local_status
  delete envelope.ack
  return envelope as OfflineOperation
}

async function updateOperations(ids: string[], update: (operation: StoredOperation) => StoredOperation): Promise<void> {
  const db = await openDatabase()
  const tx = db.transaction('operations', 'readwrite')
  const store = tx.objectStore('operations')
  for (const id of ids) {
    const operation = await requestResult<StoredOperation | undefined>(store.get(id))
    if (operation) store.put(update(operation))
  }
  await transactionDone(tx)
}

async function allFromStore<T>(name: string): Promise<T[]> {
  const db = await openDatabase()
  const tx = db.transaction(name)
  const result = await requestResult<T[]>(tx.objectStore(name).getAll())
  await transactionDone(tx)
  return result
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(databaseName, databaseVersion)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains('meta')) db.createObjectStore('meta', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('catalog_snapshots')) db.createObjectStore('catalog_snapshots', { keyPath: 'version' })
      if (!db.objectStoreNames.contains('device_session')) db.createObjectStore('device_session', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('offline_authorization')) db.createObjectStore('offline_authorization', { keyPath: 'key' })
      if (!db.objectStoreNames.contains('shift_events')) db.createObjectStore('shift_events', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('checkout_intents')) db.createObjectStore('checkout_intents', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('operations')) {
        const operations = db.createObjectStore('operations', { keyPath: 'operation_id' })
        operations.createIndex('sequence', 'sequence', { unique: true })
      }
      if (!db.objectStoreNames.contains('sync_attempts')) db.createObjectStore('sync_attempts', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('recovery_exports')) db.createObjectStore('recovery_exports', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('offline_unlock')) db.createObjectStore('offline_unlock', { keyPath: 'key' })
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('No se pudo abrir el almacenamiento local.'))
  })
}

async function getOfflineUnlockRecord(): Promise<OfflineUnlockRecord | null> {
  const db = await openDatabase()
  const tx = db.transaction('offline_unlock')
  const record = await requestResult<OfflineUnlockRecord | undefined>(tx.objectStore('offline_unlock').get('active'))
  await transactionDone(tx)
  return record ?? null
}

async function deriveUnlockVerifier(secret: string, salt: Uint8Array, iterations: number): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(secret), 'PBKDF2', false, ['deriveBits'])
  const bits = await crypto.subtle.deriveBits({ name: 'PBKDF2', hash: 'SHA-256', salt: salt as BufferSource, iterations }, key, 256)
  return new Uint8Array(bits)
}

function bytesToBase64(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
}

function base64ToBytes(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0))
}

function requestResult<T>(request: IDBRequest): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result as T)
    request.onerror = () => reject(request.error ?? new Error('Falló una operación local.'))
  })
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error ?? new Error('No se pudo confirmar la transacción local.'))
    transaction.onabort = () => reject(transaction.error ?? new Error('La transacción local fue cancelada.'))
  })
}
