import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  APIError,
  acquirePOSDevice,
  closeCashShift,
  createSale,
  getCatalogSnapshot,
  getCurrentCashShift,
  listSales,
  openCashShift,
  recordCashMovement,
  type CashMovementType,
  type CashShift,
  type CatalogSnapshot,
  type DeviceSession,
  type PaymentMethod,
  type Product,
  type Sale,
  type User,
} from './api'
import { calculateLineSubtotal, centsToInput, formatARS, parseARSCents } from './money'

interface POSDashboardProps {
  user: User
  onOpenCatalog: () => void
  onLogout: () => Promise<void>
}

type View = 'sale' | 'cash' | 'history'
type CartLine = { product: Product; quantity: string }
type PaymentDraft = {
  id: string
  method: PaymentMethod
  amount: string
  received: string
  reference: string
  verified: boolean
}

const deviceStorageKey = 'bakery-pos-device-id'

function POSDashboard({ user, onOpenCatalog, onLogout }: POSDashboardProps) {
  const [view, setView] = useState<View>('sale')
  const [device, setDevice] = useState<DeviceSession | null>(null)
  const [snapshot, setSnapshot] = useState<CatalogSnapshot | null>(null)
  const [shift, setShift] = useState<CashShift | null>(null)
  const [sales, setSales] = useState<Sale[]>([])
  const [cart, setCart] = useState<CartLine[]>([])
  const [query, setQuery] = useState('')
  const [weightDrafts, setWeightDrafts] = useState<Record<string, string>>({})
  const [payments, setPayments] = useState<PaymentDraft[]>([])
  const [checkoutOpen, setCheckoutOpen] = useState(false)
  const [lastSale, setLastSale] = useState<Sale | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [loggingOut, setLoggingOut] = useState(false)

  useEffect(() => {
    let active = true
    const deviceID = getOrCreateDeviceID()
    void Promise.all([
      acquirePOSDevice(deviceID, deviceLabel()),
      getCatalogSnapshot(),
      getCurrentCashShift(),
      listSales(),
    ])
      .then(([nextDevice, nextSnapshot, nextShift, history]) => {
        if (!active) return
        setDevice(nextDevice)
        setSnapshot(nextSnapshot)
        setShift(nextShift)
        setSales(history.items)
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

  const products = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase('es-AR')
    return (snapshot?.products ?? []).filter((product) =>
      product.active && product.sellable && product.current_price_cents !== null &&
      (!normalized || product.name.toLocaleLowerCase('es-AR').includes(normalized) || product.barcode === query.trim()),
    )
  }, [query, snapshot])

  const cartLines = useMemo(() => cart.map((line) => ({
    ...line,
    subtotal: line.product.current_price_cents === null
      ? undefined
      : calculateLineSubtotal(line.product.current_price_cents, line.product.sale_mode, line.quantity),
  })), [cart])
  const total = cartLines.reduce((sum, line) => sum + (line.subtotal ?? 0), 0)

  function addProduct(product: Product, quantity: string) {
    if (!calculateLineSubtotal(product.current_price_cents ?? 0, product.sale_mode, quantity)) {
      setError(product.sale_mode === 'weight' ? 'Ingresá un peso válido en gramos.' : 'La cantidad no es válida.')
      return
    }
    setError(null)
    setCart((current) => {
      const existing = current.find((line) => line.product.id === product.id)
      if (!existing) return [...current, { product, quantity }]
      if (product.sale_mode === 'weight') {
        const next = addExactQuantities(existing.quantity, quantity)
        return current.map((line) => line.product.id === product.id ? { ...line, quantity: next } : line)
      }
      return current.map((line) => line.product.id === product.id
        ? { ...line, quantity: String(Number(line.quantity) + Number(quantity)) }
        : line)
    })
  }

  function beginCheckout() {
    if (!shift) {
      setError('Abrí un turno de caja antes de cobrar.')
      setView('cash')
      return
    }
    if (!total || cartLines.some((line) => line.subtotal === undefined)) {
      setError('Revisá las cantidades del carrito.')
      return
    }
    const amount = centsToInput(total)
    setPayments([{ id: newID(), method: 'cash', amount, received: amount, reference: '', verified: true }])
    setCheckoutOpen(true)
    setError(null)
  }

  function submitSale(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!device || !shift || !snapshot) return
    const parsedPayments = payments.map((payment) => ({ payment, amount: parseARSCents(payment.amount) }))
    if (parsedPayments.some(({ amount }) => amount === undefined) || parsedPayments.reduce((sum, row) => sum + (row.amount ?? 0), 0) !== total) {
      setError('Las formas de pago deben sumar exactamente el total.')
      return
    }
    for (const { payment, amount } of parsedPayments) {
      const received = payment.method === 'cash' ? parseARSCents(payment.received) : undefined
      if (payment.method === 'cash' && (received === undefined || received < (amount ?? 0))) {
        setError('El efectivo recibido no alcanza para cubrir su parte del pago.')
        return
      }
    }
    const apiPayments = parsedPayments.map(({ payment, amount }) => {
      const received = payment.method === 'cash' ? parseARSCents(payment.received) : undefined
      return {
        method: payment.method,
        amount_cents: amount!,
        status: payment.method === 'cash' || payment.verified ? 'confirmed' as const : 'reported' as const,
        ...(received === undefined ? {} : { received_cents: received }),
        external_reference: payment.reference.trim(),
        verification_method: payment.method === 'cash' ? '' : payment.verified ? 'Verificación manual del operador' : '',
      }
    })
    setSubmitting(true)
    setError(null)
    void createSale({
      operation_id: newID(),
      device_session_id: device.id,
      fencing_version: device.fencing_version,
      cash_shift_id: shift.id,
      shift_version: shift.version,
      catalog_version: snapshot.version,
      items: cart.map((line) => ({ product_id: line.product.id, quantity: line.quantity })),
      payments: apiPayments,
    })
      .then((sale) => {
        setLastSale(sale)
        setSales((current) => [sale, ...current.filter((item) => item.id !== sale.id)])
        setShift((current) => current ? { ...current, version: sale.cash_shift_version, expected_cash_cents: current.expected_cash_cents + cashAmount(sale) } : current)
        setCart([])
        setCheckoutOpen(false)
      })
      .catch((caught: unknown) => setError(apiMessage(caught)))
      .finally(() => setSubmitting(false))
  }

  return (
    <main className="dashboard-shell pos-shell">
      <header className="topbar pos-topbar">
        <div className="brand-line"><span className="brand-mark">BP</span><span>Bakery POS</span><span className="section-marker">Mostrador</span></div>
        <nav className="app-nav" aria-label="Secciones principales">
          <button className={view === 'sale' ? 'active' : ''} onClick={() => setView('sale')}>Venta</button>
          <button className={view === 'cash' ? 'active' : ''} onClick={() => setView('cash')}>Caja</button>
          <button className={view === 'history' ? 'active' : ''} onClick={() => setView('history')}>Historial</button>
          <button onClick={onOpenCatalog}>Catálogo</button>
        </nav>
        <div className="user-actions">
          <span className={shift ? 'shift-chip open' : 'shift-chip'}>{shift ? `Caja abierta · v${shift.version}` : 'Caja cerrada'}</span>
          <button className="text-button" disabled={loggingOut} onClick={() => {
            setLoggingOut(true)
            void onLogout().finally(() => setLoggingOut(false))
          }}>{loggingOut ? 'Saliendo…' : user.name}</button>
        </div>
      </header>

      {error && <div className="pos-notice notice notice-error" role="alert">{error}</div>}
      {loading ? <section className="pos-loading"><span className="status-dot" />Preparando catálogo, caja y dispositivo…</section> : null}
      {!loading && view === 'sale' && (
        <SaleWorkspace
          products={products}
          query={query}
          onQuery={setQuery}
          weightDrafts={weightDrafts}
          onWeightDraft={(id, value) => setWeightDrafts((current) => ({ ...current, [id]: value }))}
          onAdd={addProduct}
          lines={cartLines}
          total={total}
          onQuantity={(id, quantity) => setCart((current) => current.map((line) => line.product.id === id ? { ...line, quantity } : line))}
          onRemove={(id) => setCart((current) => current.filter((line) => line.product.id !== id))}
          onCheckout={beginCheckout}
          shift={shift}
          lastSale={lastSale}
        />
      )}
      {!loading && view === 'cash' && <CashWorkspace device={device} shift={shift} onShift={setShift} />}
      {!loading && view === 'history' && <HistoryWorkspace sales={sales} />}

      {checkoutOpen && (
        <CheckoutPanel total={total} payments={payments} onPayments={setPayments} onClose={() => setCheckoutOpen(false)} onSubmit={submitSale} submitting={submitting} />
      )}
    </main>
  )
}

function SaleWorkspace(props: {
  products: Product[]
  query: string
  onQuery: (value: string) => void
  weightDrafts: Record<string, string>
  onWeightDraft: (id: string, value: string) => void
  onAdd: (product: Product, quantity: string) => void
  lines: Array<CartLine & { subtotal: number | undefined }>
  total: number
  onQuantity: (id: string, quantity: string) => void
  onRemove: (id: string) => void
  onCheckout: () => void
  shift: CashShift | null
  lastSale: Sale | null
}) {
  return (
    <section className="pos-layout">
      <div className="product-picker">
        <header className="pos-heading"><div><p className="eyebrow">Venta online · fase 3</p><h1>¿Qué sale hoy?</h1></div><span>{props.products.length} productos</span></header>
        <label className="pos-search"><span>Buscar o escanear</span><input autoFocus value={props.query} onChange={(event) => props.onQuery(event.target.value)} placeholder="Medialuna, pan o código…" /></label>
        <div className="product-grid">
          {props.products.map((product) => (
            <article className="sell-card" key={product.id}>
              <div><span>{product.category_name}</span><h2>{product.name}</h2><strong>{formatARS(product.current_price_cents ?? 0)} <small>/ {product.price_unit === 'kg' ? 'kg' : 'u.'}</small></strong></div>
              {product.sale_mode === 'unit' ? (
                <button type="button" onClick={() => props.onAdd(product, '1')}>+ Agregar</button>
              ) : (
                <div className="weight-add"><label><span>Gramos</span><input inputMode="decimal" value={props.weightDrafts[product.id] ?? ''} onChange={(event) => props.onWeightDraft(product.id, event.target.value)} placeholder="350" /></label><button type="button" onClick={() => props.onAdd(product, props.weightDrafts[product.id] ?? '')}>Agregar</button></div>
              )}
            </article>
          ))}
          {props.products.length === 0 && <p className="catalog-empty">No hay productos que coincidan con la búsqueda.</p>}
        </div>
      </div>
      <aside className="cart-panel">
        <header><div><p className="eyebrow">Pedido actual</p><h2>Carrito</h2></div><span>{props.lines.length}</span></header>
        <div className="cart-lines">
          {props.lines.map((line) => (
            <div className="cart-line" key={line.product.id}>
              <div><strong>{line.product.name}</strong><small>{line.product.sale_mode === 'weight' ? `${line.quantity} g` : `${line.quantity} u.`}</small></div>
              <label><span className="sr-only">Cantidad de {line.product.name}</span><input inputMode="decimal" value={line.quantity} onChange={(event) => props.onQuantity(line.product.id, event.target.value)} /></label>
              <strong>{line.subtotal === undefined ? 'Revisar' : formatARS(line.subtotal)}</strong>
              <button type="button" aria-label={`Quitar ${line.product.name}`} onClick={() => props.onRemove(line.product.id)}>×</button>
            </div>
          ))}
          {props.lines.length === 0 && <div className="empty-cart"><span>+</span><p>Elegí productos para empezar la venta.</p></div>}
        </div>
        <div className="cart-total"><span>Total</span><strong>{formatARS(props.total)}</strong></div>
        <button className="charge-button" type="button" disabled={!props.total} onClick={props.onCheckout}>{props.shift ? 'Cobrar' : 'Abrir caja para cobrar'}</button>
        {props.lastSale && <ReceiptCard sale={props.lastSale} compact />}
      </aside>
    </section>
  )
}

function CheckoutPanel({ total, payments, onPayments, onClose, onSubmit, submitting }: {
  total: number
  payments: PaymentDraft[]
  onPayments: (payments: PaymentDraft[]) => void
  onClose: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  submitting: boolean
}) {
  function patchPayment(id: string, patch: Partial<PaymentDraft>) {
    onPayments(payments.map((payment) => payment.id === id ? { ...payment, ...patch } : payment))
  }
  return (
    <div className="checkout-backdrop" role="presentation">
      <form className="checkout-sheet" onSubmit={onSubmit} aria-labelledby="checkout-title">
        <header><div><p className="eyebrow">Cobro</p><h2 id="checkout-title">Total {formatARS(total)}</h2></div><button type="button" className="panel-close" onClick={onClose}>Cerrar</button></header>
        <div className="payment-list">
          {payments.map((payment, index) => (
            <section className="payment-row" key={payment.id}>
              <span className="payment-index">0{index + 1}</span>
              <label><span>Medio</span><select value={payment.method} onChange={(event) => patchPayment(payment.id, { method: event.target.value as PaymentMethod, verified: event.target.value === 'cash' })}><option value="cash">Efectivo</option><option value="transfer">Transferencia</option><option value="qr">QR</option><option value="card">Tarjeta</option><option value="other">Otro</option></select></label>
              <label><span>Importe</span><input value={payment.amount} inputMode="decimal" onChange={(event) => patchPayment(payment.id, { amount: event.target.value })} required /></label>
              {payment.method === 'cash' ? <label><span>Recibido</span><input value={payment.received} inputMode="decimal" onChange={(event) => patchPayment(payment.id, { received: event.target.value })} required /></label> : <label><span>Referencia</span><input value={payment.reference} onChange={(event) => patchPayment(payment.id, { reference: event.target.value })} placeholder="Opcional" /></label>}
              {payment.method !== 'cash' && <label className="verify-check"><input type="checkbox" checked={payment.verified} onChange={(event) => patchPayment(payment.id, { verified: event.target.checked })} /><span>Verifiqué la acreditación externamente</span></label>}
              {payments.length > 1 && <button type="button" className="text-button" onClick={() => onPayments(payments.filter((row) => row.id !== payment.id))}>Quitar</button>}
            </section>
          ))}
        </div>
        <button type="button" className="quiet-button" onClick={() => onPayments([...payments, { id: newID(), method: 'transfer', amount: '', received: '', reference: '', verified: false }])}>+ Combinar otro medio</button>
        <footer><div><span>Debe sumar</span><strong>{formatARS(total)}</strong></div><button className="charge-button" type="submit" disabled={submitting}>{submitting ? 'Registrando…' : 'Confirmar venta'}</button></footer>
      </form>
    </div>
  )
}

function CashWorkspace({ device, shift, onShift }: { device: DeviceSession | null; shift: CashShift | null; onShift: (shift: CashShift | null) => void }) {
  const [opening, setOpening] = useState('')
  const [kind, setKind] = useState<CashMovementType>('withdrawal')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [recipient, setRecipient] = useState('')
  const [counted, setCounted] = useState('')
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function open(event: FormEvent) {
    event.preventDefault()
    const cents = parseNonNegativeCents(opening)
    if (!device || cents === undefined) return setError('Ingresá el efectivo inicial, incluso si es cero.')
    setBusy(true); setError(null)
    void openCashShift({ device_session_id: device.id, fencing_version: device.fencing_version, opening_cash_cents: cents, opening_note: '' })
      .then((next) => { onShift(next); setMessage('Turno abierto. Ya podés registrar ventas.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function movement(event: FormEvent) {
    event.preventDefault()
    const cents = parseARSCents(amount)
    if (!shift || cents === undefined) return setError('Ingresá un importe positivo.')
    setBusy(true); setError(null)
    void recordCashMovement(shift.id, { id: newID(), expected_version: shift.version, movement_type: kind, amount_cents: cents, reason, recipient })
      .then((next) => { onShift(next); setAmount(''); setReason(''); setRecipient(''); setMessage('Movimiento registrado sin alterar la cifra de ventas.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function close(event: FormEvent) {
    event.preventDefault()
    const cents = parseNonNegativeCents(counted)
    if (!shift || cents === undefined) return setError('Ingresá el efectivo contado.')
    setBusy(true); setError(null)
    void closeCashShift(shift.id, { expected_version: shift.version, counted_cash_cents: cents, closing_note: '' })
      .then((closed) => { onShift(null); setMessage(`Turno cerrado. Diferencia: ${formatSignedARS(closed.difference_cents ?? 0)}.`) })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  return (
    <section className="cash-workspace">
      <header className="pos-heading"><div><p className="eyebrow">Caja física</p><h1>{shift ? 'Turno en curso' : 'Abrir turno'}</h1></div>{shift && <div className="expected-cash"><span>Efectivo esperado</span><strong>{formatARS(shift.expected_cash_cents)}</strong><small>Versión {shift.version}</small></div>}</header>
      {error && <div className="notice notice-error" role="alert">{error}</div>}{message && <div className="notice notice-success" role="status">{message}</div>}
      {!shift ? (
        <form className="cash-card opening-card" onSubmit={open}><p>Contá el efectivo que recibe este turno. Ese importe será la base reproducible del cierre.</p><label><span>Efectivo inicial</span><input value={opening} onChange={(event) => setOpening(event.target.value)} inputMode="decimal" placeholder="30000.00" required /></label><button className="action-button" disabled={busy || !device}>Abrir caja</button></form>
      ) : (
        <div className="cash-grid">
          <form className="cash-card" onSubmit={movement}><p className="eyebrow">Movimiento manual</p><h2>Ingreso, gasto o retiro</h2><label><span>Tipo</span><select value={kind} onChange={(event) => setKind(event.target.value as CashMovementType)}><option value="withdrawal">Retiro</option><option value="expense">Gasto</option><option value="income">Ingreso excepcional</option></select></label><label><span>Importe</span><input value={amount} onChange={(event) => setAmount(event.target.value)} inputMode="decimal" required /></label><label><span>Motivo</span><input value={reason} onChange={(event) => setReason(event.target.value)} minLength={3} maxLength={300} required /></label>{kind === 'withdrawal' && <label><span>Receptor</span><input value={recipient} onChange={(event) => setRecipient(event.target.value)} minLength={2} required /></label>}<button className="action-button" disabled={busy}>Registrar movimiento</button></form>
          <form className="cash-card closing-card" onSubmit={close}><p className="eyebrow">Arqueo online</p><h2>Cerrar turno</h2><p>El sistema conservará por separado el esperado, lo contado y la diferencia. El cierre no modifica ventas históricas.</p><label><span>Efectivo contado</span><input value={counted} onChange={(event) => setCounted(event.target.value)} inputMode="decimal" required /></label><button className="quiet-button danger-button" disabled={busy}>Cerrar y guardar arqueo</button></form>
        </div>
      )}
    </section>
  )
}

function HistoryWorkspace({ sales }: { sales: Sale[] }) {
  const [selected, setSelected] = useState<Sale | null>(sales[0] ?? null)
  return <section className="history-workspace"><header className="pos-heading"><div><p className="eyebrow">Comprobantes internos</p><h1>Ventas recientes</h1></div><span>{sales.length} operaciones</span></header><div className="history-grid"><div className="sale-history-list">{sales.map((sale) => <button key={sale.id} className={selected?.id === sale.id ? 'active' : ''} onClick={() => setSelected(sale)}><span>{new Date(sale.completed_at).toLocaleString('es-AR')}</span><strong>{formatARS(sale.total_cents)}</strong><small>{sale.payment_state === 'unverified' ? 'Cobro sin verificar' : 'Cobro confirmado'}</small></button>)}{sales.length === 0 && <p className="catalog-empty">Todavía no hay ventas registradas.</p>}</div>{selected && <ReceiptCard sale={selected} />}</div></section>
}

function ReceiptCard({ sale, compact = false }: { sale: Sale; compact?: boolean }) {
  return <article className={compact ? 'receipt-card compact' : 'receipt-card'}><header><span>Comprobante interno · no fiscal</span><strong>#{sale.id.slice(0, 8)}</strong></header>{!compact && <div className="receipt-lines">{sale.items.map((item) => <div key={item.id}><span>{item.product_name} · {item.quantity} {item.base_unit === 'unit' ? 'u.' : item.base_unit}</span><strong>{formatARS(item.subtotal_cents)}</strong></div>)}</div>}<footer><span>{sale.payment_state === 'unverified' ? '⚠ Cobro sin verificar' : '✓ Cobro confirmado'}</span><strong>{formatARS(sale.total_cents)}</strong></footer>{sale.inventory_incidents.length > 0 && <p className="stock-warning">Venta permitida con {sale.inventory_incidents.length} incidencia de stock.</p>}</article>
}

function cashAmount(sale: Sale) { return sale.payments.filter((payment) => payment.method === 'cash').reduce((sum, payment) => sum + payment.amount_cents, 0) }
function apiMessage(caught: unknown) { return caught instanceof APIError || caught instanceof Error ? caught.message : 'No pudimos completar la operación.' }
function newID() { return crypto.randomUUID() }
function getOrCreateDeviceID() { const stored = localStorage.getItem(deviceStorageKey); if (stored) return stored; const id = newID(); localStorage.setItem(deviceStorageKey, id); return id }
function deviceLabel() { return `Mostrador · ${navigator.platform || 'navegador'}`.slice(0, 80) }
function parseNonNegativeCents(value: string) { if (/^0(?:\.0{1,2})?$/.test(value.trim())) return 0; return parseARSCents(value) }
function formatSignedARS(cents: number) { return `${cents < 0 ? '−' : cents > 0 ? '+' : ''}${formatARS(Math.abs(cents))}` }
function addExactQuantities(left: string, right: string) {
  const toMilliunits = (value: string) => {
    const [whole = '0', fraction = ''] = value.split('.')
    return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0'))
  }
  const sum = toMilliunits(left) + toMilliunits(right)
  const whole = sum / 1000n
  const fraction = String(sum % 1000n).padStart(3, '0').replace(/0+$/, '')
  return fraction ? `${whole}.${fraction}` : String(whole)
}

export default POSDashboard
