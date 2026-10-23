import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  APIError,
  addRecoveryDifference,
  acquirePOSDevice,
  closeCashShift,
  closeRecovery,
  forcePOSRecovery,
  getActiveRecovery,
  getCatalogSnapshot,
  getCurrentCashShift,
  getSale,
  issueOfflineAuthorization,
  listSales,
  openCashShift,
  recordCashMovement,
  reconstructRecoverySale,
  resolveRecoveryDifference,
  syncOfflineOperations,
  updateRecoveryControls,
  type CashMovementType,
  type CashShift,
  type CatalogSnapshot,
  type DeviceSession,
  type PaymentMethod,
  type Product,
  type RecoveryDetails,
  type Sale,
  type User,
} from './api'
import { calculateLineSubtotal, centsToInput, formatARS, parseARSCents } from './money'
import {
  applySyncACKs,
  blockingCollectingIntent,
  closePreparedShiftOffline,
  consolidateIntent,
  createCheckoutIntent,
  initializeOfflineStorage,
  loadPreparedContext,
  markIntentCollecting,
  markSending,
  markSyncFailed,
  openPreparedShiftOffline,
  pendingOperationCount,
  pendingOperations,
  savePreparedContext,
  type CheckoutIntent,
  type PreparedContext,
} from './offline'

interface POSDashboardProps {
  user: User
  onOpenCatalog: () => void
  onLogout: () => Promise<void>
}

type View = 'sale' | 'cash' | 'history' | 'recovery'
type CartLine = { product: Product; quantity: string }
type PaymentDraft = {
  id: string
  method: PaymentMethod
  amount: string
  received: string
  reference: string
  verified: boolean
}
type ConnectionState = 'synced' | 'offline' | 'syncing' | 'conflict' | 'recovery'

const deviceStorageKey = 'bakery-pos-device-id'
const durableLocalStorageAvailable = typeof indexedDB !== 'undefined'

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
  const [prepared, setPrepared] = useState<PreparedContext | null>(null)
  const [intent, setIntent] = useState<CheckoutIntent | null>(null)
  const [pendingCount, setPendingCount] = useState(0)
  const [connection, setConnection] = useState<ConnectionState>(navigator.onLine ? 'synced' : 'offline')
  const [recovery, setRecovery] = useState<RecoveryDetails | null>(null)

  useEffect(() => {
    let active = true
    void (durableLocalStorageAvailable ? initializeOfflineStorage() : Promise.resolve()).then(async () => {
      if (durableLocalStorageAvailable) {
        const blocking = await blockingCollectingIntent()
        const count = await pendingOperationCount()
        if (active) { setIntent(blocking); setPendingCount(count) }
      }
      const deviceID = getOrCreateDeviceID()
      try {
        const [nextDevice, nextSnapshot, nextShift, history] = await Promise.all([
          acquirePOSDevice(deviceID, deviceLabel()), getCatalogSnapshot(), getCurrentCashShift(), listSales(),
        ])
        if (!active) return
        setDevice(nextDevice)
        setSnapshot(nextSnapshot)
        setShift(nextShift)
        setSales(history.items)
        setConnection('synced')
      } catch (caught) {
        const local = await loadPreparedContext()
        if (!active) return
        if (!local) throw caught
        setPrepared(local); setDevice(local.device); setSnapshot(local.snapshot); setShift(local.shift); setConnection('offline')
      }
    }).catch((caught: unknown) => { if (active) setError(apiMessage(caught)) })
      .finally(() => { if (active) setLoading(false) })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    if (user.role !== 'owner' || !navigator.onLine) return
    void getActiveRecovery().then(setRecovery).catch(() => undefined)
  }, [user.role, connection])

  useEffect(() => {
    if (!durableLocalStorageAvailable || !device || !shift || !snapshot || !navigator.onLine) return
    let active = true
    void issueOfflineAuthorization({
      device_session_id: device.id, fencing_version: device.fencing_version,
      cash_shift_id: shift.id, shift_version: shift.version, catalog_version: snapshot.version,
    }).then(async (authorization) => {
      const context = { device, shift, snapshot, authorization }
      await savePreparedContext(context)
      if (active) setPrepared(context)
    }).catch((caught: unknown) => { if (active) setError(`No se pudo preparar el modo offline: ${apiMessage(caught)}`) })
    return () => { active = false }
  }, [device, shift, snapshot])

  useEffect(() => {
    if (!durableLocalStorageAvailable) return
    const online = () => { setConnection('syncing'); void synchronize() }
    const offline = () => setConnection('offline')
    window.addEventListener('online', online); window.addEventListener('offline', offline)
    if (navigator.onLine) void synchronize()
    return () => { window.removeEventListener('online', online); window.removeEventListener('offline', offline) }
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

  async function beginCheckout() {
    if (!shift) {
      setError('Abrí un turno de caja antes de cobrar.')
      setView('cash')
      return
    }
    if (!total || cartLines.some((line) => line.subtotal === undefined)) {
      setError('Revisá las cantidades del carrito.')
      return
    }
    if (!prepared) {
      setError('Este dispositivo todavía no completó la preparación offline. Esperá la confirmación antes de cobrar.')
      return
    }
    if (intent?.status === 'collecting') {
      setError('Hay un cobro sin consolidar. Resolvelo antes de iniciar otra venta.')
      return
    }
    try {
      const nextIntent = await createCheckoutIntent(cart.map((line) => ({ product_id: line.product.id, quantity: line.quantity })), total)
      await markIntentCollecting(nextIntent.id)
      setIntent({ ...nextIntent, status: 'collecting' })
      const amount = centsToInput(total)
      setPayments([{ id: newID(), method: 'cash', amount, received: amount, reference: '', verified: true }])
      setCheckoutOpen(true)
      setError(null)
    } catch (caught) {
      setError(`No se habilitó el cobro porque no se pudo guardar el intento: ${apiMessage(caught)}`)
    }
  }

  function submitSale(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!device || !shift || !snapshot || !prepared || !intent) return
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
        status: payment.method === 'cash' || (payment.verified && navigator.onLine) ? 'confirmed' as const : 'reported' as const,
        ...(received === undefined ? {} : { received_cents: received }),
        external_reference: payment.reference.trim(),
        verification_method: payment.method === 'cash' ? '' : payment.verified ? 'Verificación manual del operador' : '',
      }
    })
    setSubmitting(true)
    setError(null)
    const payload = {
      operation_id: newID(),
      device_session_id: device.id,
      fencing_version: device.fencing_version,
      cash_shift_id: shift.id,
      shift_version: shift.version,
      catalog_version: snapshot.version,
      items: cart.map((line) => ({ product_id: line.product.id, quantity: line.quantity })),
      payments: apiPayments,
    }
    void consolidateIntent(intent.id, payload, prepared)
      .then(async () => {
        const cashCents = apiPayments.filter((payment) => payment.method === 'cash').reduce((sum, payment) => sum + payment.amount_cents, 0)
        const nextShift = { ...shift, version: shift.version + 1, expected_cash_cents: shift.expected_cash_cents + cashCents }
        const nextPrepared = { ...prepared, shift: nextShift }
        setPrepared(nextPrepared); setShift(nextShift); await savePreparedContext(nextPrepared)
        setCart([])
        setCheckoutOpen(false)
        setIntent(null)
        setPendingCount(await pendingOperationCount())
        if (navigator.onLine) await synchronize()
        else setConnection('offline')
      })
      .catch((caught: unknown) => setError(`Cobro sin consolidar: ${apiMessage(caught)} No inicies otra venta.`))
      .finally(() => setSubmitting(false))
  }

  async function synchronize() {
    if (!navigator.onLine) { setConnection('offline'); return }
    const operations = await pendingOperations()
    if (operations.length === 0) { setPendingCount(0); setConnection('synced'); return }
    const ids = operations.map((operation) => operation.operation_id)
    setConnection('syncing')
    await markSending(ids)
    try {
      const result = await syncOfflineOperations(operations)
      await applySyncACKs(result.acks)
      if (result.acks.some((ack) => ack.status === 'recovery_evidence')) {
        setConnection('recovery')
      } else {
        const synchronizedSales = await Promise.all(result.acks.flatMap((ack) => ack.sale_id ? [getSale(ack.sale_id)] : []))
        setSales((current) => [...synchronizedSales, ...current.filter((sale) => !synchronizedSales.some((next) => next.id === sale.id))])
        if (synchronizedSales[0]) setLastSale(synchronizedSales[0])
        setConnection('synced')
      }
      setPendingCount(await pendingOperationCount())
    } catch (caught) {
      await markSyncFailed(ids)
      setPendingCount(await pendingOperationCount())
      if (caught instanceof APIError && (caught.code === 'sync_conflict' || caught.code === 'recovery_required')) setConnection('conflict')
      else setConnection(navigator.onLine ? 'conflict' : 'offline')
      setError(apiMessage(caught))
    }
  }

  return (
    <main className="dashboard-shell pos-shell">
      <header className="topbar pos-topbar">
        <div className="brand-line"><span className="brand-mark">BP</span><span>Bakery POS</span><span className="section-marker">Mostrador</span></div>
        <nav className="app-nav" aria-label="Secciones principales">
          <button className={view === 'sale' ? 'active' : ''} onClick={() => setView('sale')}>Venta</button>
          <button className={view === 'cash' ? 'active' : ''} onClick={() => setView('cash')}>Caja</button>
          <button className={view === 'history' ? 'active' : ''} onClick={() => setView('history')}>Historial</button>
          {user.role === 'owner' && <button className={view === 'recovery' ? 'active' : ''} onClick={() => setView('recovery')}>Recuperación</button>}
          <button onClick={onOpenCatalog}>Catálogo</button>
        </nav>
        <div className="user-actions">
          <button className={`sync-chip ${connection}`} onClick={() => void synchronize()}>{connectionLabel(connection, pendingCount)}</button>
          <span className={shift ? 'shift-chip open' : 'shift-chip'}>{shift ? `Caja abierta · v${shift.version}` : 'Caja cerrada'}</span>
          <button className="text-button" disabled={loggingOut} onClick={() => {
            setLoggingOut(true)
            void onLogout().finally(() => setLoggingOut(false))
          }}>{loggingOut ? 'Saliendo…' : user.name}</button>
        </div>
      </header>

      {error && <div className="pos-notice notice notice-error" role="alert">{error}</div>}
      {intent?.status === 'collecting' && !checkoutOpen && <div className="pos-notice notice notice-warning" role="alert">Cobro sin consolidar. Retomá el cobro abierto; el POS bloquea ventas nuevas para evitar perderlo.</div>}
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
          onCheckout={() => void beginCheckout()}
          shift={shift}
          lastSale={lastSale}
        />
      )}
      {!loading && view === 'cash' && <CashWorkspace device={device} shift={shift} prepared={prepared} onShift={setShift} onPrepared={setPrepared} onJournalChanged={async () => { setPendingCount(await pendingOperationCount()); if (navigator.onLine) await synchronize() }} />}
      {!loading && view === 'history' && <HistoryWorkspace sales={sales} />}
      {!loading && view === 'recovery' && user.role === 'owner' && <RecoveryWorkspace device={device} snapshot={snapshot} recovery={recovery} onRecovery={setRecovery} />}

      {checkoutOpen && (
        <CheckoutPanel total={total} payments={payments} onPayments={setPayments} onClose={() => { setCheckoutOpen(false); setError('Cobro sin consolidar. Volvé a abrirlo y confirmá la venta antes de continuar.') }} onSubmit={submitSale} submitting={submitting} offline={!navigator.onLine} />
      )}
    </main>
  )
}

function RecoveryWorkspace({ device, snapshot, recovery, onRecovery }: {
  device: DeviceSession | null
  snapshot: CatalogSnapshot | null
  recovery: RecoveryDetails | null
  onRecovery: (recovery: RecoveryDetails | null) => void
}) {
  const [reason, setReason] = useState('Pérdida o rotura del dispositivo activo')
  const [cashCount, setCashCount] = useState('')
  const [kind, setKind] = useState<RecoveryDetails['differences'][number]['kind']>('sequence_gap')
  const [differenceAmount, setDifferenceAmount] = useState('')
  const [description, setDescription] = useState('')
  const [productID, setProductID] = useState('')
  const [quantity, setQuantity] = useState('1')
  const [saleAmount, setSaleAmount] = useState('')
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('cash')
  const [sourceOperationID, setSourceOperationID] = useState('')
  const [certainty, setCertainty] = useState<'exact' | 'probable'>('probable')
  const [evidence, setEvidence] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  async function refresh() { onRecovery(await getActiveRecovery()) }

  function force(event: FormEvent) {
    event.preventDefault()
    const cents = parseNonNegativeCents(cashCount)
    if (!device || cents === undefined) return setError('Necesitás la sesión previa y el efectivo contado.')
    setBusy(true); setError(null)
    void forcePOSRecovery({ previous_session_id: device.id, device_id: getOrCreateDeviceID(), device_name: deviceLabel(), reason, cash_count_cents: cents })
      .then(refresh).then(() => setMessage('La sesión anterior quedó cercada. La conciliación está abierta.'))
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function addDifference(event: FormEvent) {
    event.preventDefault()
    if (!recovery) return
    const amount = differenceAmount.trim() ? parseNonNegativeCents(differenceAmount) : undefined
    if (differenceAmount.trim() && amount === undefined) return setError('El importe de la diferencia no es válido.')
    setBusy(true); setError(null)
    void addRecoveryDifference(recovery.id, { id: newID(), kind, ...(amount === undefined ? {} : { amount_cents: amount }), description })
      .then((next) => { onRecovery(next); setDescription(''); setDifferenceAmount(''); setMessage('Diferencia incorporada a la conciliación.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function reconstruct(event: FormEvent) {
    event.preventDefault()
    const amount = parseARSCents(saleAmount)
    if (!recovery?.affected_shift_id || !snapshot || !productID || amount === undefined) return setError('Completá producto, cantidad, importe y evidencia.')
    const operationID = newID()
    setBusy(true); setError(null)
    void reconstructRecoverySale(recovery.id, {
      reconciliation_operation_id: operationID,
      ...(sourceOperationID.trim() ? { source_operation_id: sourceOperationID.trim() } : {}),
      certainty, evidence,
      sale: {
        operation_id: operationID, device_session_id: recovery.previous_session_id,
        fencing_version: recovery.previous_fencing_version, cash_shift_id: recovery.affected_shift_id,
        shift_version: recovery.affected_shift_version, catalog_version: snapshot.version,
        items: [{ product_id: productID, quantity }],
        payments: [{ method: paymentMethod, amount_cents: amount, status: paymentMethod === 'cash' ? 'confirmed' : 'reported', external_reference: '', verification_method: '', ...(paymentMethod === 'cash' ? { received_cents: amount } : {}) }],
      },
    }).then(refresh).then(() => { setEvidence(''); setSourceOperationID(''); setSaleAmount(''); setMessage('Venta reconstruida y vinculada a la evidencia.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function saveControls(event: FormEvent) {
    event.preventDefault()
    if (!recovery) return
    setBusy(true); setError(null)
    void updateRecoveryControls(recovery.id, {
      journal_status: recovery.journal_status === 'pending' ? 'imported' : recovery.journal_status,
      gaps_documented: recovery.gaps_documented,
      digital_payments_reviewed: recovery.digital_payments_reviewed,
      inventory_counted: recovery.inventory_counted,
      owner_approved: recovery.owner_approved,
    }).then((next) => { onRecovery(next); setMessage('Controles guardados.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function resolveDifference(differenceID: string) {
    if (!recovery) return
    setBusy(true); setError(null)
    void resolveRecoveryDifference(recovery.id, differenceID)
      .then((next) => { onRecovery(next); setMessage('Diferencia marcada como resuelta.') })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  const patchControl = (patch: Partial<RecoveryDetails>) => recovery && onRecovery({ ...recovery, ...patch })
  const hasOpenDifferences = recovery?.differences.some((difference) => !difference.resolved_at) ?? false

  if (!recovery) return (
    <section className="recovery-workspace"><header className="pos-heading"><div><p className="eyebrow">Sólo dueño</p><h1>Recuperación extraordinaria</h1></div></header>
      {error && <div className="notice notice-error" role="alert">{error}</div>}
      <form className="cash-card recovery-force" onSubmit={force}><p>Cercá el puesto activo únicamente si se perdió o quedó inutilizable. Las ventas no sincronizadas deberán conciliarse.</p><label><span>Motivo verificable</span><textarea value={reason} onChange={(event) => setReason(event.target.value)} minLength={10} maxLength={500} required /></label><label><span>Efectivo contado al recuperar</span><input value={cashCount} onChange={(event) => setCashCount(event.target.value)} inputMode="decimal" required /></label><button className="danger-button" disabled={busy || !device}>Forzar recuperación</button></form>
    </section>
  )

  return (
    <section className="recovery-workspace">
      <header className="pos-heading"><div><p className="eyebrow">Conciliación trazable</p><h1>Caso #{recovery.id.slice(0, 8)}</h1><p>{recovery.reason}</p></div><div className="expected-cash"><span>Arqueo inicial</span><strong>{formatARS(recovery.cash_count_cents)}</strong><small>ACK {recovery.last_ack_sequence}</small></div></header>
      {error && <div className="notice notice-error" role="alert">{error}</div>}{message && <div className="notice notice-success" role="status">{message}</div>}
      <div className="recovery-grid">
        <form className="cash-card" onSubmit={addDifference}><p className="eyebrow">Brechas explícitas</p><h2>Registrar diferencia</h2><label><span>Tipo</span><select value={kind} onChange={(event) => setKind(event.target.value as typeof kind)}><option value="sequence_gap">Brecha de secuencia</option><option value="unknown_sale_total">Venta sin detalle</option><option value="cash">Efectivo</option><option value="digital_payment">Pago digital</option><option value="inventory">Inventario</option></select></label><label><span>Importe, si aplica</span><input value={differenceAmount} onChange={(event) => setDifferenceAmount(event.target.value)} inputMode="decimal" /></label><label><span>Descripción y evidencia</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} minLength={3} maxLength={500} required /></label><button className="action-button" disabled={busy}>Agregar diferencia</button></form>
        {recovery.affected_shift_id && <form className="cash-card" onSubmit={reconstruct}><p className="eyebrow">Venta conocida</p><h2>Reconstruir sin duplicar</h2><label><span>Producto</span><select value={productID} onChange={(event) => setProductID(event.target.value)} required><option value="">Seleccionar…</option>{snapshot?.products.filter((product) => product.sellable).map((product) => <option key={product.id} value={product.id}>{product.name}</option>)}</select></label><label><span>Cantidad exacta</span><input value={quantity} onChange={(event) => setQuantity(event.target.value)} inputMode="decimal" required /></label><label><span>Importe cobrado</span><input value={saleAmount} onChange={(event) => setSaleAmount(event.target.value)} inputMode="decimal" required /></label><label><span>Medio</span><select value={paymentMethod} onChange={(event) => setPaymentMethod(event.target.value as PaymentMethod)}><option value="cash">Efectivo</option><option value="transfer">Transferencia</option><option value="qr">QR</option><option value="card">Tarjeta</option><option value="other">Otro</option></select></label><label><span>ID original, si existe</span><input value={sourceOperationID} onChange={(event) => setSourceOperationID(event.target.value)} placeholder="UUID del diario perdido" /></label><label><span>Certeza</span><select value={certainty} onChange={(event) => setCertainty(event.target.value as typeof certainty)}><option value="exact">Exacta</option><option value="probable">Probable</option></select></label><label><span>Evidencia</span><textarea value={evidence} onChange={(event) => setEvidence(event.target.value)} minLength={3} maxLength={500} required /></label><button className="action-button" disabled={busy}>Reconstruir venta</button></form>}
        <form className="cash-card recovery-controls" onSubmit={saveControls}><p className="eyebrow">Cierre controlado</p><h2>Lista de conciliación</h2><label><span>Estado del diario</span><select value={recovery.journal_status} onChange={(event) => patchControl({ journal_status: event.target.value as RecoveryDetails['journal_status'] })}><option value="pending">Pendiente</option><option value="imported">Importado</option><option value="unrecoverable">Irrecuperable declarado</option></select></label><label className="verify-check"><input type="checkbox" checked={recovery.gaps_documented} onChange={(event) => patchControl({ gaps_documented: event.target.checked })} /><span>Brechas documentadas</span></label><label className="verify-check"><input type="checkbox" checked={recovery.digital_payments_reviewed} onChange={(event) => patchControl({ digital_payments_reviewed: event.target.checked })} /><span>Pagos digitales revisados</span></label><label className="verify-check"><input type="checkbox" checked={recovery.inventory_counted} onChange={(event) => patchControl({ inventory_counted: event.target.checked })} /><span>Inventario contado</span></label><label className="verify-check"><input type="checkbox" checked={recovery.owner_approved} onChange={(event) => patchControl({ owner_approved: event.target.checked })} /><span>Aprobación final del dueño</span></label>{hasOpenDifferences && <p className="notice notice-warning">Resolvé todas las diferencias abiertas antes del cierre final.</p>}<button className="quiet-button" disabled={busy || recovery.journal_status === 'pending'}>Guardar controles</button><button type="button" className="danger-button" disabled={busy || hasOpenDifferences || recovery.journal_status === 'pending' || !recovery.gaps_documented || !recovery.digital_payments_reviewed || !recovery.inventory_counted || !recovery.owner_approved} onClick={() => { setBusy(true); void closeRecovery(recovery.id).then((next) => { onRecovery(next); setMessage('Conciliación cerrada sin borrar evidencia.') }).catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false)) }}>Cerrar conciliación</button></form>
      </div>
      <div className="recovery-differences"><h2>Diferencias registradas</h2>{recovery.differences.length ? recovery.differences.map((item) => <article key={item.id}><strong>{item.kind.replaceAll('_', ' ')}</strong><span>{item.amount_cents === undefined ? 'Sin importe' : formatARS(item.amount_cents)}</span><p>{item.description}</p>{item.resolved_at ? <small>Resuelta el {new Date(item.resolved_at).toLocaleString('es-AR')}</small> : <button type="button" className="text-button" disabled={busy} onClick={() => resolveDifference(item.id)}>Marcar resuelta</button>}</article>) : <p>Sin diferencias documentadas todavía.</p>}</div>
    </section>
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
        <header className="pos-heading"><div><p className="eyebrow">Venta resiliente · fase 4</p><h1>¿Qué sale hoy?</h1></div><span>{props.products.length} productos</span></header>
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

function CheckoutPanel({ total, payments, onPayments, onClose, onSubmit, submitting, offline }: {
  total: number
  payments: PaymentDraft[]
  onPayments: (payments: PaymentDraft[]) => void
  onClose: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  submitting: boolean
  offline: boolean
}) {
  function patchPayment(id: string, patch: Partial<PaymentDraft>) {
    onPayments(payments.map((payment) => payment.id === id ? { ...payment, ...patch } : payment))
  }
  return (
    <div className="checkout-backdrop" role="presentation">
      <form className="checkout-sheet" onSubmit={onSubmit} aria-labelledby="checkout-title">
        <header><div><p className="eyebrow">Cobro {offline ? 'offline' : 'protegido'}</p><h2 id="checkout-title">Total {formatARS(total)}</h2></div><button type="button" className="panel-close" onClick={onClose}>Ocultar</button></header>
        {offline && <div className="notice notice-warning">Sin conexión: el cobro quedará guardado localmente. Los pagos digitales se registran como reportados.</div>}
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

function CashWorkspace({ device, shift, prepared, onShift, onPrepared, onJournalChanged }: {
  device: DeviceSession | null
  shift: CashShift | null
  prepared: PreparedContext | null
  onShift: (shift: CashShift | null) => void
  onPrepared: (context: PreparedContext) => void
  onJournalChanged: () => Promise<void>
}) {
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
    const onlineOpen = () => openCashShift({ device_session_id: device.id, fencing_version: device.fencing_version, opening_cash_cents: cents, opening_note: '' })
    const queuedOpen = async () => {
      if (!prepared) return onlineOpen()
      const transition = await openPreparedShiftOffline(prepared, cents, '')
      onPrepared(transition.context)
      return transition.context.shift
    }
    void queuedOpen()
      .then(async (next) => { onShift(next); setOpening(''); setMessage(`Turno ${prepared ? 'guardado localmente' : 'abierto'}. Ya podés registrar ventas.`); if (prepared) await onJournalChanged() })
      .catch((caught: unknown) => setError(apiMessage(caught))).finally(() => setBusy(false))
  }

  function movement(event: FormEvent) {
    event.preventDefault()
    const cents = parseARSCents(amount)
    if (!navigator.onLine) return setError('Los movimientos manuales requieren conexión en el MVP. Las ventas y el relevo de turno sí continúan offline.')
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
    const queuedClose = async () => {
      if (!prepared) return closeCashShift(shift.id, { expected_version: shift.version, counted_cash_cents: cents, closing_note: '' })
      const transition = await closePreparedShiftOffline(prepared, cents, '')
      onPrepared(transition.context)
      return transition.context.shift
    }
    void queuedClose()
      .then(async (closed) => { onShift(null); setCounted(''); setMessage(`Turno ${prepared ? 'cerrado provisionalmente' : 'cerrado'}. Diferencia: ${formatSignedARS(closed.difference_cents ?? 0)}.`); if (prepared) await onJournalChanged() })
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
          <form className="cash-card" onSubmit={movement}><p className="eyebrow">Movimiento manual · online</p><h2>Ingreso, gasto o retiro</h2><label><span>Tipo</span><select value={kind} onChange={(event) => setKind(event.target.value as CashMovementType)}><option value="withdrawal">Retiro</option><option value="expense">Gasto</option><option value="income">Ingreso excepcional</option></select></label><label><span>Importe</span><input value={amount} onChange={(event) => setAmount(event.target.value)} inputMode="decimal" required /></label><label><span>Motivo</span><input value={reason} onChange={(event) => setReason(event.target.value)} minLength={3} maxLength={300} required /></label>{kind === 'withdrawal' && <label><span>Receptor</span><input value={recipient} onChange={(event) => setRecipient(event.target.value)} minLength={2} required /></label>}<button className="action-button" disabled={busy || !navigator.onLine}>Registrar movimiento</button></form>
          <form className="cash-card closing-card" onSubmit={close}><p className="eyebrow">Arqueo resiliente</p><h2>Cerrar turno</h2><p>Si no hay conexión, el cierre queda provisional en el diario y la próxima apertura se encadena detrás.</p><label><span>Efectivo contado</span><input value={counted} onChange={(event) => setCounted(event.target.value)} inputMode="decimal" required /></label><button className="quiet-button danger-button" disabled={busy}>Cerrar y guardar arqueo</button></form>
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

function connectionLabel(state: ConnectionState, pending: number) {
  if (state === 'syncing') return 'Sincronizando…'
  if (state === 'conflict') return 'Conflicto · revisar'
  if (state === 'recovery') return 'Recuperación en curso'
  if (state === 'offline') return `Sin conexión · ${pending} pendiente${pending === 1 ? '' : 's'}`
  return pending ? `${pending} pendiente${pending === 1 ? '' : 's'}` : 'Sincronizado'
}

export default POSDashboard
