# Panadería POS — Especificación funcional y técnica del MVP

**Versión:** 1.0 (baseline de alcance)  
**Fecha:** 11 de octubre de 2026  
**Estado:** Preparado para planificación y desarrollo; requiere validación con el dueño antes del uso productivo.  
**Negocio inicial:** una panadería, una caja física operativa, un solo dispositivo autorizado para registrar ventas a la vez.

> **Propósito:** permitir vender rápidamente desde celular, tablet o computadora, gestionar caja por turnos, controlar productos terminados y materias primas, registrar compras y producción, y funcionar con ventas offline, sin comprar hardware nuevo para comenzar.

## 1. Decisiones confirmadas

1. Aplicación web responsive instalable como PWA; venta desde celular, tablet o computadora.
2. Un solo dispositivo autorizado para cobrar a la vez. El dispositivo puede cambiar luego de sincronizar las operaciones pendientes.
3. Balanza digital existente **sin impresora**: peso ingresado manualmente en el MVP. Lector de códigos opcional para seleccionar cualquier producto compatible. **Todos** los productos se pueden seleccionar también desde pantalla, incluidos bebidas y envasados.
4. Ventas por unidad o peso, con botones rápidos, categorías, buscador, códigos y pagos combinados.
5. Medios de pago: efectivo, transferencia, Mercado Pago/QR, tarjeta de débito o crédito. Registrar pago no significa procesarlo: el sistema no confirma fondos en proveedores externos.
6. Caja por turnos, con responsable, apertura, retiros parciales de efectivo, entrega entre turnos, arqueos y cierres; los retiros no alteran la cifra de ventas.
7. Cualquier empleado autorizado a vender puede corregir/anular/devolver ventas ya cobradas con motivo obligatorio y trazabilidad; no requiere autorización del dueño.
8. En cada devolución se elige: apto para reincorporar al stock o descarte/no apto. Aplicar restricciones de seguridad alimentaria.
9. Si una venta supera el stock teórico: advertir, **permitir vender**, conservar stock negativo/diferencia y abrir incidencia de inventario.
10. Compras: ingreso rápido de mercadería y registro completo con proveedor, ítems, cantidades y costos; ambos modifican el mismo inventario.
11. Controlar materias primas, elaborados y reventa; registrar producción diaria. Recetas exactas o aproximadas, con consumos sugeridos editables, inventarios periódicos y ajustes auditables.
12. Manejar sobrantes conservados, descartados y reutilizados; lotes/vencimientos **selectivos** para productos que lo necesiten.
13. Importación inicial de catálogo desde lista en papel/fotos, mediante revisión humana y planilla CSV; luego alta/edición manual y masiva.
14. Antes del lanzamiento: **conteo físico completo al final de una jornada**, con saldos iniciales a una hora de corte; primera venta en sistema al día siguiente. Registrar cualquier movimiento sucedido entre el conteo y la apertura.
15. El cierre de venta registra la operación y muestra total; comprobante **interno no fiscal opcional**. Integración con ARCA y emisión fiscal quedan fuera del MVP; las obligaciones fiscales existentes deben atenderse por separado.
16. Ventas offline en el dispositivo activo con persistencia local; sincronización posterior, protección contra duplicados y pendientes visibles. Operaciones administrativas/compra/producción preferentemente online en MVP.

## 2. Alcance y exclusiones

### 2.1 Módulos dentro del MVP

- Autenticación, usuarios, roles y permisos.
- Catálogo, categorías, unidades, códigos de barras opcionales, precios y cambios masivos con previsualización.
- POS: búsqueda, botones táctiles, lector compatible, carrito, venta por gramos/unidades, pago simple o combinado, historial, comprobante interno opcional.
- Caja: turnos, apertura, movimientos, retiros, entrega, arqueo, cierres e incidencias.
- Inventario: saldos, movimientos inmutables, ajustes, conteos físicos, faltantes, alertas, lotes selectivos.
- Compras y proveedores; registro rápido o compra detallada.
- Recetas, producción, consumo de ingredientes, costos estimados, producción sin receta completa marcada como incompleta.
- Mermas, descartes, conservación de sobrantes, reutilización apta.
- Devoluciones, anulaciones y correcciones con auditabilidad.
- Reportes operativos básicos por día/turno/medio de pago, inventario, producción y costos estimados.
- Offline POS con cola de sincronización y recuperación de errores.
- Asistente de configuración inicial e importación CSV.

### 2.2 Fuera del MVP

- Facturación electrónica/ARCA, comprobantes fiscales, elección arbitraria de montos a facturar.
- Cobros procesados automáticamente por APIs de Mercado Pago, adquirentes o bancos; conciliación automática bancaria.
- Balanza integrada, impresión de etiquetas por peso, impresora térmica como requisito.
- Varias cajas cobrando simultáneamente, múltiples sucursales y transferencia online de stock entre ellas.
- Contabilidad integral, sueldos, impuestos, pronósticos de demanda e inteligencia artificial sobre recetas.
- Control exhaustivo de lote de **todos** los ingredientes/productos, inventario completamente editable offline.

### 2.3 Supuestos explícitos

- Internet disponible habitualmente; modo offline para contingencias de corta duración.
- Un solo puesto de cobro activo, aunque otros dispositivos puedan consultar el panel administrativo estando online.
- Precios en pesos argentinos (ARS); venta minorista por unidad o por peso.
- Las recetas pueden ser aproximadas: costos y consumos también pueden ser estimados.
- Los datos iniciales provienen de papel/fotos y se revisan antes de importar; no se requiere OCR en el producto.

## 3. Experiencia de usuario: pantallas y flujos

### 3.1 Navegación principal

1. **Ventas** (pantalla de inicio para empleados).
2. **Caja y turnos** (apertura, retiros, arqueo, entrega, cierre).
3. **Historial** (búsqueda de ventas, comprobante interno, correcciones, anulaciones, devoluciones).
4. **Productos y precios** (alta, edición, categorías, importación, aumento masivo).
5. **Inventario** (saldos, alertas, conteo físico, ajustes, lotes, mermas).
6. **Compras y proveedores** (carga rápida, compra completa).
7. **Producción y recetas** (producción diaria, ingredientes, costos).
8. **Reportes** (ventas por turno/medio de pago, diferencias, stock, desperdicio).
9. **Empleados y configuración** (usuarios, permisos, dispositivos, parámetros).

### 3.2 Flujo de venta

1. Verificar usuario autenticado, turno abierto y permiso del dispositivo activo.
2. Seleccionar producto por botón/buscador/lector. Todo producto puede elegirse manualmente.
3. Para producto por peso: solicitar gramos leídos de la balanza actual; calcular subtotal. Para unidad: ingresar cantidad.
4. Construir carrito; permitir eliminar/cambiar ítems antes del cobro.
5. Si stock teórico menor que la cantidad: advertencia no bloqueante y marca de posible diferencia.
6. Seleccionar uno o varios medios de pago; validar que las asignaciones sumen el total. Separar monto recibido y vuelto del **importe neto** aplicado a efectivo.
7. Confirmar operación: online se envía a API; offline se escribe primero en IndexedDB como evento pendiente y se muestra estado «guardado localmente».
8. Mostrar total y confirmación. De forma opcional abrir comprobante **interno no fiscal**.
9. No registrar automáticamente como confirmado un pago digital si no se comprobó por el mecanismo externo utilizado.

### 3.3 Flujo caja y cambio de turno

- Apertura con responsable y conteo del efectivo inicial.
- Durante el turno: ingresos en efectivo por ventas, salidas por gastos/devoluciones, retiros parciales e ingresos excepcionales; cada movimiento con motivo cuando corresponda.
- Retiro: efectivo contado `300.000`, dejar `100.000`, retiro `200.000`. Se registra monto, responsable, receptor, fecha/hora; **no** disminuye ventas.
- Arqueo: comparar saldo esperado con saldo contado y registrar diferencia.
- Cierre: responsable cierra su turno, se registra entrega y el siguiente turno se abre con el efectivo realmente recibido. La caja física no depende del dispositivo.
- Si hay operaciones pendientes offline: cierre **provisional** en el mismo dispositivo, seguido por sincronización ordenada; no emitir reporte definitivo ni autorizar cambio de dispositivo hasta terminar de sincronizar.
- Cierres históricos son inmutables; ajustes tardíos se imputan en la sesión vigente y se vinculan a la operación histórica.

**Efectivo esperado = efectivo inicial + ingresos netos de efectivo + ingresos manuales − gastos en efectivo − retiros − reintegros en efectivo.**

Las ventas digitales se informan separadamente. Ingreso de compra pagado desde caja y egreso manual deben referenciarse para no duplicar la salida.

### 3.4 Compras y producción

- **Carga rápida:** seleccionar insumo/producto, cantidad, motivo y, opcionalmente, costo/proveedor. Crear un movimiento de ingreso.
- **Compra completa:** proveedor, fecha, líneas, cantidades, costos, total y estado del pago. Al marcar recepción se suman existencias una sola vez.
- **Receta:** producto final, rendimiento de lote, ingredientes y cantidades de referencia, modalidad `exacta`/`aproximada`.
- **Producción:** introducir cantidad real obtenida; mostrar y permitir editar consumos sugeridos donde corresponda; confirmar suma del terminado, consumo de materias primas y costo estimado. Si no se conoce consumo, guardar incidencia de información incompleta en vez de inventarlo.
- **Sobrantes:** registrar conservación, descarte o reasignación a otra producción; la reutilización consume stock del producto original y crea nuevo terminado según la elaboración.
- **Lotes:** productos configurados como perecederos o almacenados de un día a otro mantienen fecha de elaboración/vencimiento cuando sea conocida; los demás pueden usar stock agregado. Priorizar productos que vencen primero cuando corresponda, respetando restricciones de aptitud.

### 3.5 Devoluciones y anulaciones

- Cualquier empleado que vende puede iniciar una devolución/anulación y debe informar motivo.
- Límite: no devolver más cantidad/dinero de lo efectivamente vendido menos devoluciones ya confirmadas.
- Registrar producto devuelto como `apto para stock` o `no apto/descarte`; productos alimenticios solo vuelven a estar disponibles cuando corresponda por condiciones sanitarias.
- Devolución digital: debe constar confirmación del reintegro por el canal externo. No fingir reintegro automático.
- Reversar efectos con nuevos movimientos referenciados, nunca editar destructivamente el movimiento histórico.
- **Offline:** no se permiten devoluciones/anulaciones de operaciones sincronizadas hasta recuperar conexión. Se permite editar el carrito antes del cobro sin conexión.

## 4. Arquitectura técnica

### 4.1 Stack propuesto

- **Frontend:** React + TypeScript + Vite, interfaz mobile-first y táctil, PWA.
- **Offline:** service worker para caché de la aplicación; IndexedDB para catálogo, turno local, operaciones pendientes y estados de sincronización.
- **Backend:** Go, API REST, monolito modular (no microservicios).
- **Persistencia:** PostgreSQL, migraciones SQL versionadas.
- **Infra de desarrollo:** Docker Compose para PostgreSQL, backend y frontend local; variables por entorno.
- **Producción:** HTTPS, frontend estático y backend/API con base de datos persistente y respaldos. Elegir hosting tras validar costo y confiabilidad; no exigir despliegue pago en la fase de desarrollo.

### 4.2 Módulos Go

`auth`, `employees`, `devices`, `catalog`, `pricing`, `pos`, `payments`, `cash`, `inventory`, `purchasing`, `recipes`, `production`, `returns`, `reports`, `sync`, `audit`.

Separar rutas/controladores, casos de uso, reglas de dominio y repositorios; definir interfaces solo donde aporten pruebas o desacople real. Compartir la misma base de datos mediante límites de transacción explícitos.

### 4.3 Persistencia y precisión

- IDs UUID; hora `TIMESTAMPTZ` en UTC, visualización en `America/Argentina/Cordoba`; reportes por día comercial local.
- Precios y pagos en **centavos enteros** (`BIGINT`) o `NUMERIC` exacto; nunca `float` para importes monetarios.
- Cantidades normalizadas en unidades base (`g`, `ml`, `unidad`) con `NUMERIC(18,3)` cuando haga falta precisión; validar cantidades enteras para artículos por unidad.
- La venta almacena **snapshot** del nombre, unidad, precio y tasa de conversión aplicados; cambiar el catálogo no modifica históricos.
- Cálculo de venta al peso: subtotal = redondear(`precio_centavos_por_kg × gramos / 1000`, a centavos), regla de redondeo consistente y testeada en backend y frontend.
- Inventario: libro de movimientos inmutable + saldo proyectado/transaccional. Nunca sobrescribir un saldo sin su movimiento correspondiente.
- Costos estimados: calcular desde costos conocidos de ingredientes con una política documentada (inicialmente último costo conocido o promedio configurado); distinguir costo faltante de costo cero. No llamar «ganancia neta» a margen sin mano de obra y gastos indirectos.

## 5. Modelo de datos conceptual

| Entidad/tabla | Campos principales y relaciones |
|---|---|
| `users`, `roles` | id, nombre, credencial, rol, activo; contraseñas/PIN con hash seguro |
| `devices`, `pos_device_sessions` | dispositivo, estado activo, token/fencing/version, usuario, timestamps, excepciones de recuperación |
| `cash_shifts` | responsable, apertura, cierre, efectivo inicial, esperado, contado, diferencia, estado provisional/definitivo |
| `cash_movements` | turno, tipo (retiro, gasto, ingreso, reintegro), centavos, motivo, emisor/receptor, venta/ref opcional |
| `categories`, `products`, `barcodes` | nombre, tipo (`ingredient`, `produced`, `resale`), venta por unidad/peso, unidad, flags de lote, activo |
| `product_prices`, `price_change_audit` | precio, unidad, vigencia, quién modificó, cambio masivo |
| `suppliers`, `purchases`, `purchase_items` | proveedor, costo, estado de recepción/pago, detalle |
| `recipes`, `recipe_items` | producto final, rendimiento, ingredientes, cantidades, versión, modo exacto/aproximado |
| `production_batches`, `production_consumptions` | producto obtenido, cantidad, receta versión, consumos reales/estimados, costo y operador |
| `inventory_lots` | producto, fecha elaboración, vencimiento, estado, cantidad vigente (si se controla por lote) |
| `inventory_movements`, `inventory_balances` | evento de entrada/salida/ajuste y saldo materializado, referencia a origen/turno/lote |
| `stock_counts`, `stock_count_items` | conteo, fecha de corte, cantidad registrada, cantidad física, diferencia, confirmación |
| `sales`, `sale_items` | ID único, turno, dispositivo, totales, instantánea de precios/pesos, estado, timestamps |
| `sale_payments` | venta, medio, monto, estado reportado/confirmado, comprobante externo opcional |
| `returns`, `return_items` | venta original, cantidades/importes, causa, reintegro, aptitud, movimiento compensatorio |
| `stock_incidents`, `audit_events` | diferencias, estado de resolución, actor, antes/después, evento relacionado |
| `sync_operations` | `operation_id`, dispositivo, secuencia, payload hash, estado y resultado guardado |

**Restricciones esenciales:**

- `UNIQUE(device_id, operation_id)` para deduplicar sincronización; mismo ID con payload distinto = error.
- Referencias de venta original y control transaccional para evitar reintegros acumulados por encima de lo cobrado.
- Una sola sesión activa de POS por panadería, gestionada desde el servidor; sin conexión los demás dispositivos no pueden adquirir una nueva sesión.
- Transacciones SQL para confirmar ventas, movimientos, pagos, saldos, incidencias y resultado idempotente como una unidad.
- Claves foráneas, índices sobre (`created_at`, `shift_id`, `product_id`, `status`) según consultas reales.
- Los eventos del inventario y auditoría no se eliminan físicamente.

## 6. Contratos de API iniciales

Convención: `/api/v1`, JSON, errores uniformes `{ code, message, details, request_id }`, paginación y filtros. Autorización por rol/operación; toda operación mutante con contexto de usuario y auditoría.

| Método | Endpoint | Uso |
|---|---|---|
| `POST` | `/auth/login` | Inicio de sesión |
| `GET` | `/me` | Usuario/rol/sesión actual |
| `GET, POST` | `/products` | Listado y alta |
| `GET, PATCH` | `/products/{id}` | Detalle y modificación |
| `POST` | `/products/import/preview` | Validar CSV antes de importar |
| `POST` | `/products/import/commit` | Confirmar importación revisada |
| `POST` | `/prices/bulk-preview` | Simular aumento masivo |
| `POST` | `/prices/bulk-commit` | Confirmar precios |
| `GET` | `/pos/catalog-snapshot` | Catálogo/versionado para offline |
| `POST` | `/pos/device/acquire` | Habilitar puesto activo online |
| `POST` | `/pos/device/release` | Liberar luego de sincronizar |
| `POST` | `/cash/shifts` | Abrir turno |
| `GET` | `/cash/shifts/current` | Turno vigente |
| `POST` | `/cash/shifts/{id}/movements` | Retiro/gasto/ingreso |
| `POST` | `/cash/shifts/{id}/close` | Arqueo y cierre |
| `POST` | `/sales` | Venta online idempotente |
| `GET` | `/sales` | Historial |
| `GET` | `/sales/{id}` | Detalle/comprobante interno |
| `POST` | `/sales/{id}/returns` | Devolución total/parcial |
| `POST` | `/sales/{id}/void` | Anulación justificada |
| `POST` | `/sync/operations` | Sincronizar lote ordenado de eventos POS |
| `GET` | `/sync/status` | Estado de operaciones pendientes/conocidas |
| `GET` | `/inventory/balances` | Existencias y alertas |
| `GET` | `/inventory/movements` | Kardex/historial |
| `POST` | `/inventory/counts` | Iniciar inventario físico |
| `POST` | `/inventory/counts/{id}/confirm` | Confirmar saldos iniciales/ajustes |
| `POST` | `/purchases` | Compra completa |
| `POST` | `/inventory/receipts` | Entrada rápida |
| `GET, POST` | `/recipes` | Listar/crear recetas |
| `POST` | `/production/batches` | Confirmar producción |
| `POST` | `/inventory/waste` | Merma, descarte, reutilización |
| `GET` | `/reports/daily-sales` | Resumen de ventas y pagos |
| `GET` | `/reports/cash-shifts` | Cierres por turno |
| `GET` | `/reports/stock-incidents` | Diferencias e incidencias |

Los nombres son propuesta inicial; formalizar solicitudes/respuestas y contratos OpenAPI antes de implementar cada módulo.

## 7. Protocolo offline y cambio de dispositivo

### 7.1 Estructura de cola local

Cada venta local guarda `operation_id` UUID, `device_id`, `sequence`, `shift_id`, `created_at_local`, `catalog_version`, `payment_status`, `sale_payload`, `checksum`, `sync_status` (`pending`, `sending`, `acknowledged`, `conflict`, `failed`). Escribir la operación **en una transacción IndexedDB** antes de mostrarla como guardada.

Estados visibles: `Sincronizado`, `Sin conexión — N pendientes`, `Sincronizando`, `Error — revisión requerida`.

### 7.2 Algoritmo servidor

1. Recibir lote ordenado por `device_id` + `sequence`, con ID estable por operación.
2. Verificar dispositivo habilitado, identidad, turno, estructura, rangos y hash.
3. Buscar `sync_operations(device_id, operation_id)`:
   - procesada con mismo hash → responder mismo resultado;
   - ID con hash diferente → conflicto;
   - nueva → ejecutar en una transacción SQL.
4. Insertar venta + líneas + pagos + movimientos de inventario/caja + alertas e incidencia de stock, actualizar saldos, registrar ACK.
5. Si queda stock negativo, **no rechazar** por esa razón: crear incidencia para conciliar después.
6. Responder ACK por operación; el frontend marca como sincronizada y conserva historial local el tiempo necesario. Si hay error recuperable, reintentar; si es conflicto no recuperable, detener secuencia y solicitar resolución.

### 7.3 Limitaciones y medidas de seguridad

- La cola local **no es respaldo garantizado**: IndexedDB puede perderse si se borran datos del navegador, se daña el dispositivo o el sistema operativo expulsa almacenamiento. Solicitar almacenamiento persistente donde esté disponible, monitorear cuota y hacer pruebas reales de los dispositivos elegidos.
- Cachear shell/catálogo antes del primer corte; no prometer primer uso offline en un dispositivo nunca configurado.
- Sincronizar al abrir la app, al detectar conectividad efectiva con la API y mediante acción manual; Background Sync si existe como mejora, **nunca como única estrategia**.
- Pago en efectivo puede quedar registrado localmente como cobrado en caja física y pendiente de consolidación central; pago digital offline exige confirmación externa explícita antes de marcarlo acreditado.
- Mantener secuencia del turno si se abre/cierra offline en **el mismo dispositivo**: cierre provisional, apertura local del siguiente turno y sincronización ordenada. Si no se puede reconciliar, intervención del dueño.
- Cambio normal de dispositivo: conexión activa, cola local en cero, liberación registrada en servidor, adquisición desde nuevo dispositivo y refresco de catálogo/turno.
- Recuperación forzada por pérdida/rotura: operación extraordinaria auditada, **riesgo explícito de ventas no sincronizadas**, conciliación física de caja y stock. No afirmar exclusión absoluta cuando hay un dispositivo previamente desconectado.

## 8. Roles y autorización

| Acción | Empleado vendedor | Encargado / dueño |
|---|---|---|
| Venta y medios de pago | Sí | Sí |
| Abrir/cerrar turno asignado, retiro con motivo | Sí | Sí |
| Corregir, anular y devolver con motivo | Sí | Sí |
| Ver precios y existencias | Sí | Sí |
| Registrar producción | Según permiso operativo | Sí |
| Ingreso/ajuste manual de inventario | Según permiso | Sí |
| Editar precios, importar catálogo | No | Sí |
| Ver reportes globales, gestionar usuarios | No | Sí |
| Forzar recuperación de dispositivo | No | Sí |

Registrar `actor_id` y `shift_id` en cada operación. Las claves/PIN se almacenan con hash adecuado; controlar intentos de autenticación, expiración de sesión, permisos en servidor y secretos fuera del repositorio.

## 9. Plan de implementación y entregables

| Iteración | Entregable demostrable | Criterio para avanzar |
|---|---|---|
| 0. Fundaciones | Monorepo (frontend/backend), Docker Compose, migraciones, CI, login y roles, fixtures demo | Entorno local reproducible, test de auth y permisos |
| 1. Catálogo y precios | CRUD, importación CSV con previsualización, unidades, códigos, aumento masivo | Productos visibles en POS, precios con historial |
| 2. POS y caja | Carrito, gr/kg, pagos combinados, turno, retiros y cierre, comprobante interno | Efectivo esperado y contado reproducible; no duplicar dinero |
| 3. Offline y auditoría | IndexedDB, service worker, sincronización idempotente, control dispositivo activo, incidencias | Pruebas de corte, reintento, cambio de dispositivo, recuperación |
| 4. Inventario, compras y producción | Movimientos, conteos, recetas, producción, lotes selectivos, mermas y devolución | Stocks consistentes por transacción; trazabilidad completa |
| 5. Reportes y puesta en marcha | Reportes, backups/restauración, importación real, ensayo integral y capacitación | Checklist de aceptación aprobado por dueño y desarrollador |

Orden técnico: conviene implementar primero modelos de inventario y auditoría mínimos para que las ventas de la iteración 2 ya registren movimientos correctamente; las interfaces completas de compras/producción pueden llegar en la 4.

**Infraestructura mínima de desarrollo:** `.env.example`, comandos `make dev`, `make test`, `make migrate`, bases de prueba separadas, semillas demo. **Seguridad de producción:** HTTPS, contraseñas hasheadas, backups automáticos y ensayo de recuperación, control de acceso, registros y monitoreo de errores.

## 10. Pruebas de aceptación obligatorias

1. Vender 350 g de producto a ARS 9.500/kg → total ARS 3.325 y movimiento de −350 g (si hay existencias insuficientes, venta + incidencia).
2. Modificar precio después de una venta → venta histórica conserva valor original.
3. Venta mixta ARS 10.000 con ARS 4.000 efectivo + ARS 6.000 QR → los pagos suman el total; en caja solo ingresan ARS 4.000 netos en efectivo.
4. Abrir turno con ARS 30.000, vender ARS 120.000 en efectivo, registrar gasto ARS 20.000 y retiro ARS 40.000 → efectivo esperado ARS 90.000.
5. Retirar ARS 200.000 de ARS 300.000 y dejar ARS 100.000 → retirar disminuye efectivo de caja, **no ventas**.
6. Cambiar responsable de turno → historial conserva entrega, recepción y diferencias.
7. Crear receta aproximada → sugerencia de consumo editable; registrar producción aumenta elaborado y descuenta consumos confirmados.
8. Registrar lote de producto perecedero y descarte → salida auditada sin venta.
9. Devolver gaseosa apta → stock aumenta según decisión y caja refleja devolución si fue efectivo; devolver alimento no apto → no vuelve al stock vendible.
10. Un empleado anula venta con motivo → no desaparece venta original y los movimientos se compensan una sola vez.
11. Intentar sincronizar dos veces `operation_id` → una sola venta, mismo ACK.
12. Cortar Internet con PWA inicializada → venta queda local; reabrir app con datos locales disponibles y recuperar red → se sincroniza sin duplicar.
13. Intentar cambiar dispositivo con pendientes locales → cambio normal bloqueado; recuperar conexión, sincronizar y cambiar.
14. Comprobar stock teórico 0,2 kg y venta 0,5 kg → advertencia, venta permitida, saldo −0,3 kg e incidencia pendiente.
15. Cierre offline de un turno y apertura posterior en **mismo dispositivo** → eventos sincronizados en orden, cierres provisionales consolidados sin modificar saldos históricos arbitrariamente.
16. Importación CSV con producto duplicado o peso/importe inválido → previsualización muestra errores y evita importación parcial silenciosa.
17. Stock inicial completo con fecha/hora de corte → saldo proveniente de conteo; los movimientos posteriores se aplican una sola vez.
18. Backup restaurado en ambiente de prueba → ventas, caja y stock coinciden.

## 11. Puesta en marcha real

1. Reunir fotos/cuadernos → transcribir a CSV con categoría, tipo, unidad, precio; el dueño verifica importes.
2. Configurar roles, empleados, precio por kg/unidad, proveedores, recetas disponibles, productos por lote.
3. Cargar catálogo y probar POS/lector si lo hubiese; precargar PWA y verificar offline en **el dispositivo elegido**.
4. Simular ventas, pagos, devoluciones, corte de conexión, cambio de turno y arqueo en un entorno de pruebas, separado del real.
5. Al finalizar una jornada: congelar una hora de corte y **contar físicamente todo** (materias primas, terminados y reventa). Crear movimiento de saldo inicial por producto/lote donde corresponda; incluir explícitamente ceros contados.
6. Si se produce, compra o mueve mercadería después del conteo, registrar ese movimiento con su hora y origen antes de la primera venta al día siguiente.
7. Al día siguiente: abrir primer turno, contar efectivo inicial, activar el puesto de cobro, trabajar normalmente.
8. Al cierre: verificar caja, operaciones pendientes de sync, diferencias de inventario y errores registrados.
9. Mantener plan de contingencia manual ante pérdida de datos/dispositivo y mecanismo documentado de conciliación posterior.

## 12. Riesgos, definiciones operativas y puntos a validar

- **Operación:** confirmar nombre de negocio, horarios reales de turnos, cantidad/nombres de empleados, catálogos y balanza exacta (marca/modelo). Ninguno bloquea diseño de dominio.
- **Offline:** pérdida de almacenamiento local, cambio forzado de dispositivo, pagos digitales no verificados, catálogo/precios desactualizados. Probar específicamente Android/iOS/PC usados en el comercio.
- **Control de alimentos:** aptitud de reventa, conservación, fecha/lote y reutilización deben cumplir reglas sanitarias aplicables. La aplicación no decide automáticamente qué alimento es seguro.
- **Facturación:** integración ARCA fuera de alcance. Un comprobante interno no sustituye un comprobante fiscal; el negocio debe mantener su proceso de emisión obligatorio por medios autorizados. No automatizar montos arbitrarios de facturación.
- **Ganancias:** la primera versión informa ingresos, costos **estimados** y márgenes parciales; no equiparar esto a beneficio neto o contabilidad fiscal.
- **Implementación:** antes del primer despliegue real, seleccionar hosting/backup/monitorización acordes al uso crítico de caja y ensayar una recuperación completa.

## 13. Decisión de cierre del alcance

Este documento es la **baseline v1.0**: cualquier nueva necesidad se evalúa como corrección imprescindible o mejora posterior. El desarrollo comienza con el flujo de productos → POS → caja → stock, incorpora offline temprano y termina cuando compras, recetas, producción, reportes y pruebas integrales pasan los criterios de aceptación. El dueño valida los flujos con datos reales antes del uso productivo.

### Referencias técnicas de soporte

- MDN — PWA, operación offline y Service Workers: https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Offline_and_background_operation
- MDN — Storage API y persistencia: https://developer.mozilla.org/en-US/docs/Web/API/Storage_API
- web.dev — Offline data en PWA: https://web.dev/learn/pwa/offline-data
- PostgreSQL — Numeric types y precisión: https://www.postgresql.org/docs/current/datatype-numeric.html

## 14. Cierre de decisiones de fase 0

La fase 0 fue cerrada el 11 de octubre de 2026 sin modificar el alcance
funcional del MVP. Las ambigüedades de implementación, máquinas de estados,
recuperación de dispositivo, conciliación offline y criterios ampliados de
aceptación se documentan en:

- [Cierre de fase 0](./FASE_0_CIERRE_DE_DECISIONES.md)
- [Registro de decisiones de arquitectura](./adr/README.md)
- [Máquinas de estados](./MAQUINAS_DE_ESTADO_MVP.md)
- [Protocolo offline y recuperación](./PROTOCOLO_OFFLINE_Y_RECUPERACION.md)
- [Plan de implementación actualizado](./PLAN_IMPLEMENTACION_MVP.md)
- [Pruebas críticas](./PRUEBAS_CRITICAS_MVP.md)

Estos documentos refinan la baseline. Cualquier cambio futuro que contradiga el
alcance funcional requiere una decisión explícita y aprobación previa.
