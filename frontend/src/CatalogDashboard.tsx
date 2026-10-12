import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  APIError,
  commitBulkPrices,
  commitProductImport,
  createCategory,
  createProduct,
  listCategories,
  listProducts,
  previewBulkPrices,
  previewProductImport,
  updateProduct,
  type BaseUnit,
  type BulkPricePreview,
  type Category,
  type ImportPreview,
  type Product,
  type ProductInput,
  type ProductType,
  type SaleMode,
  type User,
} from './api'
import { centsToInput, formatARS, parseARSCents } from './money'

interface CatalogDashboardProps {
  user: User
  onLogout: () => Promise<void>
  onOpenPOS?: () => void
}

type Panel = 'product' | 'bulk' | 'import' | null

function CatalogDashboard({ user, onLogout, onOpenPOS }: CatalogDashboardProps) {
  const [categories, setCategories] = useState<Category[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(true)
  const [loggingOut, setLoggingOut] = useState(false)
  const [query, setQuery] = useState('')
  const [panel, setPanel] = useState<Panel>(null)
  const [editing, setEditing] = useState<Product | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const canManage = user.permissions.includes('pricing:write')

  function loadCatalog(search = '') {
    setLoading(true)
    setError(null)
    void Promise.all([listCategories(), listProducts(search)])
      .then(([nextCategories, nextProducts]) => {
        setCategories(nextCategories)
        setProducts(nextProducts)
      })
      .catch((caught: unknown) => setError(apiMessage(caught)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    let active = true
    void Promise.all([listCategories(), listProducts()])
      .then(([nextCategories, nextProducts]) => {
        if (!active) return
        setCategories(nextCategories)
        setProducts(nextProducts)
      })
      .catch((caught: unknown) => {
        if (active) setError(apiMessage(caught))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const activeProducts = useMemo(() => products.filter((product) => product.active), [products])
  const pricedProducts = useMemo(
    () => activeProducts.filter((product) => product.current_price_cents !== null),
    [activeProducts],
  )

  function closePanel() {
    setPanel(null)
    setEditing(null)
  }

  return (
    <main className="dashboard-shell catalog-shell">
      <header className="topbar">
        <div className="brand-line">
          <span className="brand-mark">BP</span>
          <span>Bakery POS</span>
          <span className="section-marker">Catálogo</span>
        </div>
        <div className="user-actions">
          {onOpenPOS && <button className="quiet-button compact-button" type="button" onClick={onOpenPOS}>Ir a ventas</button>}
          <span className="role-chip">{user.role === 'owner' ? 'Dueño' : 'Empleado'}</span>
          <button
            className="text-button"
            type="button"
            disabled={loggingOut}
            onClick={() => {
              setLoggingOut(true)
              void onLogout().finally(() => setLoggingOut(false))
            }}
          >
            {loggingOut ? 'Saliendo…' : 'Cerrar sesión'}
          </button>
        </div>
      </header>

      <section className="catalog-content">
        <header className="catalog-hero">
          <div>
            <p className="eyebrow">Fase 2 · productos y precios</p>
            <h1>Buen día, {user.name}.</h1>
            <p>Un catálogo corto de leer, rápido de mantener y listo para alimentar el futuro mostrador.</p>
          </div>
          <dl className="catalog-stats" aria-label="Resumen del catálogo">
            <div><dt>Activos</dt><dd>{activeProducts.length}</dd></div>
            <div><dt>Con precio</dt><dd>{pricedProducts.length}</dd></div>
            <div><dt>Categorías</dt><dd>{categories.filter((category) => category.active).length}</dd></div>
          </dl>
        </header>

        {error && <div className="notice notice-error" role="alert">{error}</div>}
        {message && <div className="notice notice-success" role="status">{message}</div>}

        <div className="catalog-toolbar">
          <form
            className="search-box"
            role="search"
            onSubmit={(event) => {
              event.preventDefault()
              loadCatalog(query)
            }}
          >
            <label htmlFor="catalog-search">Buscar producto o código</label>
            <div>
              <input
                id="catalog-search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Ej. medialuna o 779…"
              />
              <button type="submit">Buscar</button>
            </div>
          </form>
          {canManage && (
            <div className="catalog-actions">
              <button type="button" className="quiet-button" onClick={() => setPanel('import')}>Importar CSV</button>
              <button type="button" className="quiet-button" onClick={() => setPanel('bulk')}>Aumento masivo</button>
              <button
                type="button"
                className="action-button"
                onClick={() => {
                  setEditing(null)
                  setPanel('product')
                }}
              >
                + Nuevo producto
              </button>
            </div>
          )}
        </div>

        {!canManage && (
          <aside className="read-only-note">
            <strong>Vista de empleado</strong>
            <span>Podés consultar precios y productos; las modificaciones están reservadas al dueño.</span>
          </aside>
        )}

        {panel === 'product' && canManage && (
          <ProductEditor
            categories={categories}
            product={editing}
            onClose={closePanel}
            onCategoryCreated={(category) => setCategories((current) => [...current, category])}
            onSaved={(product) => {
              setProducts((current) => {
                const exists = current.some((item) => item.id === product.id)
                return exists ? current.map((item) => item.id === product.id ? product : item) : [...current, product]
              })
              setMessage(editing ? 'Producto actualizado con historial conservado.' : 'Producto creado y listo en el catálogo.')
              closePanel()
            }}
          />
        )}

        {panel === 'bulk' && canManage && (
          <BulkPricePanel
            onClose={closePanel}
            onCommitted={() => {
              setMessage('Aumento aplicado en una sola operación; los precios anteriores siguen en el historial.')
              closePanel()
              loadCatalog(query)
            }}
          />
        )}

        {panel === 'import' && canManage && (
          <ImportPanel
            onClose={closePanel}
            onCommitted={(count) => {
              setMessage(`${count} productos importados sin filas parciales.`)
              closePanel()
              loadCatalog(query)
            }}
          />
        )}

        <section className="product-section" aria-labelledby="products-title">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Listado operativo</p>
              <h2 id="products-title">Productos</h2>
            </div>
            <span>{products.length} resultados</span>
          </div>
          {loading ? (
            <p className="catalog-empty">Actualizando catálogo…</p>
          ) : products.length === 0 ? (
            <p className="catalog-empty">Todavía no hay productos. {canManage ? 'Creá el primero o importá un CSV revisado.' : ''}</p>
          ) : (
            <div className="product-list">
              {products.map((product, index) => (
                <article className={!product.active ? 'product-row product-row-inactive' : 'product-row'} key={product.id}>
                  <span className="product-number">{String(index + 1).padStart(2, '0')}</span>
                  <div className="product-identity">
                    <span>{product.category_name}</span>
                    <h3>{product.name}</h3>
                    <small>{product.barcode || 'Sin código'} · {productTypeLabel(product.product_type)}</small>
                  </div>
                  <div className="product-measure">
                    <span>{product.sale_mode === 'weight' ? 'Por peso' : 'Por unidad'}</span>
                    <strong>{product.track_lots ? 'Con lotes' : 'Stock agregado'}</strong>
                  </div>
                  <div className="product-price">
                    <span>{product.sellable ? 'Precio vigente' : 'No vendible'}</span>
                    <strong>{product.current_price_cents === null ? '—' : formatARS(product.current_price_cents)}</strong>
                    {product.price_unit && <small>por {product.price_unit === 'kg' ? 'kg' : 'unidad'}</small>}
                  </div>
                  {canManage && (
                    <div className="row-actions">
                      <button
                        type="button"
                        className="text-button"
                        onClick={() => {
                          setEditing(product)
                          setPanel('product')
                        }}
                      >Editar</button>
                      <button
                        type="button"
                        className="text-button muted-action"
                        onClick={() => {
                          setError(null)
                          void updateProduct(product.id, { active: !product.active })
                            .then((updated) => setProducts((current) => current.map((item) => item.id === updated.id ? updated : item)))
                            .catch((caught: unknown) => setError(apiMessage(caught)))
                        }}
                      >{product.active ? 'Desactivar' : 'Activar'}</button>
                    </div>
                  )}
                </article>
              ))}
            </div>
          )}
        </section>
      </section>
    </main>
  )
}

interface ProductEditorProps {
  categories: Category[]
  product: Product | null
  onClose: () => void
  onCategoryCreated: (category: Category) => void
  onSaved: (product: Product) => void
}

function ProductEditor({ categories, product, onClose, onCategoryCreated, onSaved }: ProductEditorProps) {
  const [name, setName] = useState(product?.name ?? '')
  const [categoryID, setCategoryID] = useState(product?.category_id ?? categories.find((category) => category.active)?.id ?? '')
  const [productType, setProductType] = useState<ProductType>(product?.product_type ?? 'produced')
  const [saleMode, setSaleMode] = useState<SaleMode>(product?.sale_mode ?? 'unit')
  const [baseUnit, setBaseUnit] = useState<BaseUnit>(product?.base_unit ?? 'unit')
  const [sellable, setSellable] = useState(product?.sellable ?? true)
  const [trackLots, setTrackLots] = useState(product?.track_lots ?? false)
  const [barcode, setBarcode] = useState(product?.barcode ?? '')
  const [price, setPrice] = useState(product?.current_price_cents == null ? '' : centsToInput(product.current_price_cents))
  const [newCategory, setNewCategory] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    let priceCents: number | undefined
    if (sellable) {
      priceCents = parseARSCents(price)
      if (priceCents === undefined) {
        setError('Ingresá un precio positivo con hasta dos decimales.')
        return
      }
    }
    if (!categoryID) {
      setError('Creá o seleccioná una categoría.')
      return
    }
    const input: ProductInput = {
      category_id: categoryID,
      name,
      product_type: productType,
      sale_mode: saleMode,
      base_unit: baseUnit,
      sellable,
      track_lots: trackLots,
      barcode,
      ...(priceCents === undefined ? {} : { price_cents: priceCents }),
    }
    setSubmitting(true)
    const operation = product ? updateProduct(product.id, input) : createProduct(input)
    void operation.then(onSaved).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setSubmitting(false))
  }

  return (
    <section className="work-panel" aria-labelledby="product-editor-title">
      <header>
        <div><p className="eyebrow">Edición controlada</p><h2 id="product-editor-title">{product ? 'Editar producto' : 'Nuevo producto'}</h2></div>
        <button type="button" className="panel-close" onClick={onClose}>Cerrar</button>
      </header>
      {error && <div className="notice notice-error" role="alert">{error}</div>}
      <form className="editor-grid" onSubmit={handleSubmit}>
        <label className="wide-field"><span>Nombre</span><input value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required /></label>
        <label><span>Categoría</span><select value={categoryID} onChange={(event) => setCategoryID(event.target.value)} required><option value="">Seleccionar</option>{categories.filter((category) => category.active).map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</select></label>
        <div className="inline-create">
          <label><span>Nueva categoría</span><input value={newCategory} onChange={(event) => setNewCategory(event.target.value)} maxLength={80} /></label>
          <button
            type="button"
            className="quiet-button"
            disabled={!newCategory.trim()}
            onClick={() => {
              void createCategory(newCategory).then((category) => {
                onCategoryCreated(category)
                setCategoryID(category.id)
                setNewCategory('')
              }).catch((caught: unknown) => setError(apiMessage(caught)))
            }}
          >Crear</button>
        </div>
        <label><span>Tipo</span><select value={productType} onChange={(event) => setProductType(event.target.value as ProductType)}><option value="produced">Elaborado</option><option value="resale">Reventa</option><option value="ingredient">Materia prima</option></select></label>
        <label><span>Modalidad</span><select value={saleMode} onChange={(event) => { const mode = event.target.value as SaleMode; setSaleMode(mode); if (sellable) setBaseUnit(mode === 'weight' ? 'g' : 'unit') }}><option value="unit">Por unidad</option><option value="weight">Por peso</option></select></label>
        <label><span>Unidad base</span><select value={baseUnit} disabled={sellable} onChange={(event) => setBaseUnit(event.target.value as BaseUnit)}><option value="unit">Unidad</option><option value="g">Gramo</option><option value="ml">Mililitro</option></select></label>
        <label><span>Precio ARS {saleMode === 'weight' ? 'por kg' : 'por unidad'}</span><input inputMode="decimal" value={price} disabled={!sellable} onChange={(event) => setPrice(event.target.value)} placeholder="9500.00" required={sellable} /></label>
        <label><span>Código opcional</span><input value={barcode} onChange={(event) => setBarcode(event.target.value)} maxLength={64} /></label>
        <div className="check-fields wide-field">
          <label><input type="checkbox" checked={sellable} onChange={(event) => { const checked = event.target.checked; setSellable(checked); if (checked) setBaseUnit(saleMode === 'weight' ? 'g' : 'unit') }} /> Disponible para venta</label>
          <label><input type="checkbox" checked={trackLots} onChange={(event) => setTrackLots(event.target.checked)} /> Controlar por lotes</label>
        </div>
        <div className="panel-actions wide-field"><button type="button" className="quiet-button" onClick={onClose}>Cancelar</button><button type="submit" className="action-button" disabled={submitting}>{submitting ? 'Guardando…' : 'Guardar producto'}</button></div>
      </form>
    </section>
  )
}

function BulkPricePanel({ onClose, onCommitted }: { onClose: () => void; onCommitted: () => void }) {
  const [percentage, setPercentage] = useState('')
  const [preview, setPreview] = useState<BulkPricePreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  return (
    <section className="work-panel" aria-labelledby="bulk-title">
      <header><div><p className="eyebrow">Previsualización obligatoria</p><h2 id="bulk-title">Aumento masivo</h2></div><button type="button" className="panel-close" onClick={onClose}>Cerrar</button></header>
      <p className="panel-intro">El porcentaje se aplica a todos los productos activos con precio. Nada cambia hasta confirmar la vista previa.</p>
      {error && <div className="notice notice-error" role="alert">{error}</div>}
      <div className="bulk-controls"><label><span>Porcentaje de aumento</span><input value={percentage} onChange={(event) => { setPercentage(event.target.value); setPreview(null) }} inputMode="decimal" placeholder="10.00" /></label><button type="button" className="quiet-button" disabled={busy} onClick={() => { setBusy(true); setError(null); void previewBulkPrices(percentage).then(setPreview).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false)) }}>Previsualizar</button></div>
      {preview && <><div className="preview-list">{preview.items.map((item) => <div key={item.product_id}><span>{item.product_name}</span><del>{formatARS(item.old_price_cents)}</del><strong>{formatARS(item.new_price_cents)}</strong></div>)}</div><div className="panel-actions"><button type="button" className="quiet-button" onClick={onClose}>Cancelar</button><button type="button" className="action-button" disabled={busy} onClick={() => { setBusy(true); void commitBulkPrices(preview).then(onCommitted).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false)) }}>Confirmar {preview.items.length} cambios</button></div></>}
    </section>
  )
}

function ImportPanel({ onClose, onCommitted }: { onClose: () => void; onCommitted: (count: number) => void }) {
  const [csv, setCSV] = useState('')
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  return (
    <section className="work-panel" aria-labelledby="import-title">
      <header><div><p className="eyebrow">Importación atómica</p><h2 id="import-title">Revisar CSV</h2></div><button type="button" className="panel-close" onClick={onClose}>Cerrar</button></header>
      <p className="panel-intro">Columnas: name, category, product_type, sale_mode, base_unit, sellable, track_lots, barcode, price_ars.</p>
      {error && <div className="notice notice-error" role="alert">{error}</div>}
      <label className="file-field"><span>Archivo CSV</span><input type="file" accept=".csv,text/csv" onChange={(event) => { const file = event.target.files?.[0]; setPreview(null); if (file) void file.text().then(setCSV).catch(() => setError('No pudimos leer el archivo.')) }} /></label>
      <button type="button" className="quiet-button" disabled={!csv || busy} onClick={() => { setBusy(true); setError(null); void previewProductImport(csv).then(setPreview).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false)) }}>Previsualizar archivo</button>
      {preview && <><div className={preview.error_count ? 'import-summary import-summary-error' : 'import-summary'}><strong>{preview.valid_count} válidas</strong><span>{preview.error_count} con errores</span></div><div className="import-rows">{preview.rows.map((row) => <div key={row.row_number}><span>Fila {row.row_number}</span><strong>{row.name || 'Sin nombre'}</strong><small>{row.errors.length ? row.errors.join(' · ') : 'Lista para importar'}</small></div>)}</div><div className="panel-actions"><button type="button" className="quiet-button" onClick={onClose}>Cancelar</button><button type="button" className="action-button" disabled={busy || preview.error_count > 0} onClick={() => { setBusy(true); void commitProductImport(csv, preview.preview_hash).then((result) => onCommitted(result.imported_count)).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false)) }}>Importar {preview.valid_count} productos</button></div></>}
    </section>
  )
}

function apiMessage(error: unknown): string {
  return error instanceof APIError ? error.message : 'No pudimos completar la operación.'
}

function productTypeLabel(type: ProductType): string {
  if (type === 'ingredient') return 'Materia prima'
  if (type === 'resale') return 'Reventa'
  return 'Elaborado'
}

export default CatalogDashboard
