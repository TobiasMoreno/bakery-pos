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
