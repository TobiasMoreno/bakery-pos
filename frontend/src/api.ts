export type Role = 'seller' | 'owner'

export interface User {
  id: string
  name: string
  username: string
  role: Role
  permissions: string[]
}

export type ProductType = 'ingredient' | 'produced' | 'resale'
export type SaleMode = 'unit' | 'weight'
export type BaseUnit = 'unit' | 'g' | 'ml'

export interface Category {
  id: string
  name: string
  active: boolean
}

export interface Product {
  id: string
  category_id: string
  category_name: string
  name: string
  product_type: ProductType
  sale_mode: SaleMode
  base_unit: BaseUnit
  sellable: boolean
  track_lots: boolean
  active: boolean
  barcode: string | null
  current_price_cents: number | null
  price_unit: 'unit' | 'kg' | null
  created_at: string
  updated_at: string
}

export interface ProductInput {
  category_id: string
  name: string
  product_type: ProductType
  sale_mode: SaleMode
  base_unit: BaseUnit
  sellable: boolean
  track_lots: boolean
  barcode: string
  price_cents?: number
}

export interface BulkPriceItem {
  product_id: string
  product_name: string
  old_price_cents: number
  new_price_cents: number
  price_unit: 'unit' | 'kg'
}

export interface BulkPricePreview {
  percentage: string
  preview_hash: string
  items: BulkPriceItem[]
}

export interface ImportRow {
  row_number: number
  name: string
  category: string
  barcode: string
  errors: string[]
}

export interface ImportPreview {
  preview_hash: string
  valid_count: number
  error_count: number
  rows: ImportRow[]
}

export type InventoryMovementType =
  | 'initial'
  | 'purchase'
  | 'production'
  | 'sale'
  | 'return'
  | 'waste'
  | 'adjustment'

export interface InventoryBalance {
  product_id: string
  product_name: string
  base_unit: BaseUnit
  quantity: string
  updated_at: string
}

export interface InventoryMovement {
  id: string
  product_id: string
  product_name: string
  base_unit: BaseUnit
  quantity_delta: string
  movement_type: InventoryMovementType
  reference_type: string
  reference_id: string | null
  actor_id: string
  request_id: string
  reason?: string
  occurred_at: string
  created_at: string
}

export interface Page<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

export interface CatalogSnapshot {
  version: number
  generated_at: string
  products: Product[]
}

export interface DeviceSession {
  id: string
  device_id: string
  device_name: string
  user_id: string
  status: 'active' | 'released' | 'recovery_required'
  fencing_version: number
  acquired_at: string
}

export interface CashShift {
  id: string
  status: 'open' | 'closed' | 'provisional_closed' | 'reconciliation_required'
  version: number
  device_session_id: string
  opened_by: string
  opening_cash_cents: number
  opening_note?: string
  opened_at: string
  counted_cash_cents?: number
  expected_cash_cents: number
  difference_cents?: number
  closing_note?: string
  closed_at?: string
}

export type CashMovementType = 'income' | 'expense' | 'withdrawal'
export type PaymentMethod = 'cash' | 'transfer' | 'qr' | 'card' | 'other'
export type PaymentStatus = 'reported' | 'confirmed'

export interface SaleInput {
  operation_id: string
  device_session_id: string
  fencing_version: number
  cash_shift_id: string
  shift_version: number
  catalog_version: number
  items: Array<{ product_id: string; quantity: string }>
  payments: Array<{
    method: PaymentMethod
    amount_cents: number
    status: PaymentStatus
    received_cents?: number
    external_reference: string
    verification_method: string
  }>
}

export interface OfflineAuthorization {
  token: string
  user_id: string
  device_id: string
  device_session_id: string
  fencing_version: number
  cash_shift_id: string
  shift_version: number
  catalog_version: number
  issued_at: string
  expires_at: string
}

export interface OfflineCloseShiftInput {
  cash_shift_id: string
  expected_version: number
  counted_cash_cents: number
  closing_note: string
}

export interface OfflineOpenShiftInput {
  id: string
  previous_cash_shift_id: string
  device_session_id: string
  fencing_version: number
  opening_cash_cents: number
  opening_note: string
}

export type OfflinePayload = SaleInput | OfflineCloseShiftInput | OfflineOpenShiftInput

export interface OfflineOperation {
  operation_id: string
  operation_type: 'sale' | 'shift_close' | 'shift_open'
  device_id: string
  device_session_id: string
  fencing_version: number
  sequence: number
  created_at_local: string
  timezone: string
  previous_hash: string
  payload_hash: string
  authorization: string
  payload: OfflinePayload
}

export interface SyncACK {
  operation_id: string
  sequence: number
  payload_hash: string
  status: 'acknowledged' | 'recovery_evidence'
  sale_id?: string
  shift_id?: string
  replayed: boolean
}

export interface RecoveryDifference {
  id: string
  kind: 'unknown_sale_total' | 'cash' | 'digital_payment' | 'inventory' | 'sequence_gap'
  amount_cents?: number
  description: string
  created_at: string
  resolved_at?: string
}

export interface RecoveryDetails {
  id: string
  previous_session_id: string
  replacement_session_id: string
  affected_shift_id?: string
  status: 'open' | 'resolved'
  reason: string
  last_ack_sequence: number
  last_ack_hash: string
  cash_count_cents: number
  opened_at: string
  previous_fencing_version: number
  affected_shift_version: number
  replacement_session: DeviceSession
  journal_status: 'pending' | 'imported' | 'unrecoverable'
  gaps_documented: boolean
  digital_payments_reviewed: boolean
  inventory_counted: boolean
  owner_approved: boolean
  differences: RecoveryDifference[]
}

export interface Sale {
  id: string
  cash_shift_id: string
  payment_state: 'unverified' | 'settled' | 'issue'
  status: string
  total_cents: number
  completed_at: string
  cash_shift_version: number
  replayed: boolean
  items: Array<{
    id: string
    product_id: string
    product_name: string
    sale_mode: SaleMode
    base_unit: BaseUnit
    quantity: string
    unit_price_cents: number
    subtotal_cents: number
  }>
  payments: Array<{
    id: string
    method: PaymentMethod
    amount_cents: number
    status: PaymentStatus
    received_cents?: number
    change_cents?: number
  }>
  inventory_incidents: Array<{
    id: string
    product_id: string
    balance_quantity: string
    status: string
  }>
}

interface APIErrorBody {
  code?: string
  message?: string
  request_id?: string
}

export class APIError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
    readonly requestID?: string,
  ) {
    super(message)
  }
}

async function readError(response: Response): Promise<APIError> {
  let body: APIErrorBody = {}
  try {
    body = (await response.json()) as APIErrorBody
  } catch {
    // The fallback below intentionally hides unexpected upstream bodies.
  }
  return new APIError(
    body.message ?? 'No pudimos completar la operación.',
    response.status,
    body.code,
    body.request_id,
  )
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    ...init,
    headers: init?.body
      ? { 'Content-Type': 'application/json', ...init.headers }
      : init?.headers,
  })
  if (!response.ok) throw await readError(response)
  return (await response.json()) as T
}

export async function getCurrentUser(): Promise<User | null> {
  const response = await fetch('/api/v1/me', { credentials: 'include' })
  if (response.status === 401) return null
  if (!response.ok) throw await readError(response)
  return (await response.json()) as User
}

export async function login(username: string, password: string): Promise<User> {
  const response = await fetch('/api/v1/auth/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!response.ok) throw await readError(response)
  return (await response.json()) as User
}

export async function logout(): Promise<void> {
  const response = await fetch('/api/v1/auth/logout', {
    method: 'POST',
    credentials: 'include',
  })
  if (!response.ok && response.status !== 401) throw await readError(response)
}

export async function listCategories(): Promise<Category[]> {
  const response = await request<{ items: Category[] }>('/api/v1/categories')
  return response.items
}

export async function createCategory(name: string): Promise<Category> {
  return request<Category>('/api/v1/categories', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export async function listProducts(query = ''): Promise<Product[]> {
  const params = new URLSearchParams({ limit: '200' })
  if (query.trim()) params.set('q', query.trim())
  const response = await request<{ items: Product[] }>(`/api/v1/products?${params.toString()}`)
  return response.items
}

export async function createProduct(input: ProductInput): Promise<Product> {
  return request<Product>('/api/v1/products', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export async function updateProduct(id: string, input: Partial<ProductInput> & { active?: boolean }): Promise<Product> {
  return request<Product>(`/api/v1/products/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(input),
  })
}

export async function previewBulkPrices(percentage: string): Promise<BulkPricePreview> {
  return request<BulkPricePreview>('/api/v1/prices/bulk-preview', {
    method: 'POST',
    body: JSON.stringify({ percentage, product_ids: [] }),
  })
}

export async function commitBulkPrices(preview: BulkPricePreview): Promise<BulkPricePreview> {
  return request<BulkPricePreview>('/api/v1/prices/bulk-commit', {
    method: 'POST',
    body: JSON.stringify({
      percentage: preview.percentage,
      product_ids: [],
      preview_hash: preview.preview_hash,
    }),
  })
}

export async function previewProductImport(csv: string): Promise<ImportPreview> {
  return request<ImportPreview>('/api/v1/products/import/preview', {
    method: 'POST',
    body: JSON.stringify({ csv }),
  })
}

export async function commitProductImport(csv: string, previewHash: string): Promise<{ imported_count: number }> {
  return request<{ imported_count: number }>('/api/v1/products/import/commit', {
    method: 'POST',
    body: JSON.stringify({ csv, preview_hash: previewHash }),
  })
}

export async function listInventoryBalances(query = ''): Promise<Page<InventoryBalance>> {
  const params = new URLSearchParams({ limit: '200' })
  if (query.trim()) params.set('q', query.trim())
  return request<Page<InventoryBalance>>(`/api/v1/inventory/balances?${params.toString()}`)
}

export async function listInventoryMovements(productID?: string): Promise<Page<InventoryMovement>> {
  const params = new URLSearchParams({ limit: '200' })
  if (productID) params.set('product_id', productID)
  return request<Page<InventoryMovement>>(`/api/v1/inventory/movements?${params.toString()}`)
}

export async function getCatalogSnapshot(): Promise<CatalogSnapshot> {
  return request<CatalogSnapshot>('/api/v1/pos/catalog-snapshot')
}

export async function acquirePOSDevice(deviceID: string, deviceName: string): Promise<DeviceSession> {
  return request<DeviceSession>('/api/v1/pos/device/acquire', {
    method: 'POST',
    body: JSON.stringify({ device_id: deviceID, device_name: deviceName }),
  })
}

export async function releasePOSDevice(session: DeviceSession): Promise<void> {
  const response = await fetch('/api/v1/pos/device/release', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: session.id, fencing_version: session.fencing_version, pending_operations: 0 }),
  })
  if (!response.ok) throw await readError(response)
}

export async function getCurrentCashShift(): Promise<CashShift | null> {
  const response = await request<{ shift: CashShift | null }>('/api/v1/cash/shifts/current')
  return response.shift
}

export async function openCashShift(input: {
  device_session_id: string
  fencing_version: number
  opening_cash_cents: number
  opening_note: string
}): Promise<CashShift> {
  return request<CashShift>('/api/v1/cash/shifts', { method: 'POST', body: JSON.stringify(input) })
}

export async function recordCashMovement(shiftID: string, input: {
  id: string
  expected_version: number
  movement_type: CashMovementType
  amount_cents: number
  reason: string
  recipient: string
}): Promise<CashShift> {
  return request<CashShift>(`/api/v1/cash/shifts/${shiftID}/movements`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export async function closeCashShift(shiftID: string, input: {
  expected_version: number
  counted_cash_cents: number
  closing_note: string
}): Promise<CashShift> {
  return request<CashShift>(`/api/v1/cash/shifts/${shiftID}/close`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export async function createSale(input: SaleInput): Promise<Sale> {
  return request<Sale>('/api/v1/sales', { method: 'POST', body: JSON.stringify(input) })
}

export async function getSale(id: string): Promise<Sale> {
  return request<Sale>(`/api/v1/sales/${id}`)
}

export async function issueOfflineAuthorization(input: {
  device_session_id: string
  fencing_version: number
  cash_shift_id: string
  shift_version: number
  catalog_version: number
}): Promise<OfflineAuthorization> {
  return request<OfflineAuthorization>('/api/v1/pos/offline-authorizations', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export async function syncOfflineOperations(operations: OfflineOperation[]): Promise<{ acks: SyncACK[] }> {
  return request<{ acks: SyncACK[] }>('/api/v1/sync/operations', {
    method: 'POST',
    body: JSON.stringify({ operations }),
  })
}

export async function getActiveRecovery(): Promise<RecoveryDetails | null> {
  return (await request<{ recovery: RecoveryDetails | null }>('/api/v1/pos/recovery/current')).recovery
}

export async function forcePOSRecovery(input: {
  previous_session_id: string
  device_id: string
  device_name: string
  reason: string
  cash_count_cents: number
}): Promise<unknown> {
  return request('/api/v1/pos/recovery/force', { method: 'POST', body: JSON.stringify(input) })
}

export async function addRecoveryDifference(caseID: string, input: {
  id: string
  kind: RecoveryDifference['kind']
  amount_cents?: number
  description: string
}): Promise<RecoveryDetails> {
  return request<RecoveryDetails>(`/api/v1/pos/recovery/${caseID}/differences`, { method: 'POST', body: JSON.stringify(input) })
}

export async function resolveRecoveryDifference(caseID: string, differenceID: string): Promise<RecoveryDetails> {
  return request<RecoveryDetails>(`/api/v1/pos/recovery/${caseID}/differences/${differenceID}/resolve`, { method: 'POST' })
}

export async function updateRecoveryControls(caseID: string, input: {
  journal_status: RecoveryDetails['journal_status']
  gaps_documented: boolean
  digital_payments_reviewed: boolean
  inventory_counted: boolean
  owner_approved: boolean
}): Promise<RecoveryDetails> {
  return request<RecoveryDetails>(`/api/v1/pos/recovery/${caseID}/controls`, { method: 'PUT', body: JSON.stringify(input) })
}

export async function closeRecovery(caseID: string): Promise<RecoveryDetails> {
  return request<RecoveryDetails>(`/api/v1/pos/recovery/${caseID}/close`, { method: 'POST' })
}

export async function reconstructRecoverySale(caseID: string, input: {
  reconciliation_operation_id: string
  source_operation_id?: string
  certainty: 'exact' | 'probable'
  evidence: string
  sale: SaleInput
}): Promise<Sale> {
  return request<Sale>(`/api/v1/pos/recovery/${caseID}/reconstructions`, { method: 'POST', body: JSON.stringify(input) })
}

export async function listSales(): Promise<Page<Sale>> {
  return request<Page<Sale>>('/api/v1/sales?limit=30')
}
