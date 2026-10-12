# ADR-0003: Sincronización offline y recuperación forzada conciliable

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

Una venta puede haberse cobrado físicamente y existir sólo en IndexedDB. Si el
dispositivo se pierde o se fuerza el traspaso antes de sincronizar, rechazar sus
operaciones por una nueva versión de cercado evitaría duplicados, pero también
podría ocultar ventas reales.

Ningún sistema puede reconstruir automáticamente el detalle cuya única copia
fue destruida antes de sincronizar. La garantía alcanzable es que ninguna
operación recuperada sea descartada o aplicada dos veces y que toda brecha quede
visible hasta una conciliación auditada.

## Decisión

Antes de habilitar el cobro, el POS debe persistir un `checkout_intent` con ID,
carrito, total, turno y estado `ready_for_payment`. Al iniciar la recepción de
dinero se cambia a `collecting` en otra transacción local. Sólo después se puede
pedir o aceptar el pago.

Cada operación POS local finalizada tendrá:

- `operation_id` UUID estable;
- `device_id` y `device_session_id`;
- `fencing_version`;
- `sequence` monotónica por sesión;
- `created_at_local` y zona horaria;
- `catalog_version` y `shift_id`;
- payload canónico, `payload_hash` y `previous_hash`;
- estado local `pending`, `sending`, `acknowledged`, `conflict` o `failed`.

La venta final, el registro de la cola y la resolución del `checkout_intent` se
escriben en una única transacción IndexedDB antes de mostrar “guardado
localmente”. Si esa consolidación falla después de iniciar el cobro, el intent
queda `collecting`, la interfaz muestra “cobro sin consolidar” y bloquea nuevas
ventas hasta resolverlo. El hash encadenado detecta huecos o corrupción; no se
considera una firma de seguridad.

El servidor aplica estas restricciones:

- `UNIQUE(device_id, operation_id)`;
- `UNIQUE(device_session_id, sequence)`;
- mismo ID y mismo hash devuelve el ACK almacenado;
- mismo ID o secuencia con otro hash produce conflicto;
- cada operación nueva se procesa en una transacción SQL independiente;
- el lote se procesa en orden y se detiene en el primer conflicto no
  recuperable.

Los ACK no eliminan inmediatamente el diario local. Se conserva el historial de
la sesión mientras haya espacio y, como mínimo, durante 90 días. Se solicita
almacenamiento persistente y se monitorea cuota.

## Caso de recuperación

Una recuperación forzada crea un `device_recovery_case` que conserva:

- sesión anterior y último `sequence`/hash reconocido por el servidor;
- motivo, dueño responsable y timestamps;
- arqueo inicial de efectivo en el momento de recuperar;
- revisión de comprobantes digitales;
- estado de la importación del diario anterior;
- entradas reconstruidas, diferencias e incidencias;
- resolución y aprobación final.

El nuevo dispositivo puede continuar operando después de abrir el caso, contar
la caja y abrir un nuevo turno vinculado. El caso anterior no se considera
resuelto por ese hecho.

## Conciliación manual segura e idempotente

1. Si el dispositivo reaparece, sólo puede entrar en modo recuperación y
   exportar/sincronizar su diario; no puede volver a vender.
2. Cada operación importada se valida contra la secuencia, cadena de hashes y
   ACK conocidos.
3. Una venta reconstruida manualmente usa un
   `reconciliation_operation_id` único y el mismo pipeline transaccional de una
   venta normal.
4. Cuando se conoce el `source_operation_id`, se registra el vínculo antes de
   aplicar efectos. Sus constraints impiden aplicar la operación original y la
   reconstruida dos veces.
5. Sin ID original, se buscan candidatos por sesión, número local de
   comprobante, hora, total y medios de pago. Una coincidencia ambigua nunca se
   resuelve automáticamente; requiere decisión del dueño.
6. Si llega después una operación original equivalente, se vincula como
   evidencia y su ACK apunta al resultado ya contabilizado.
7. Si difiere de la reconstrucción, no se reemplaza historia. Se registran
   movimientos delta compensatorios y la decisión del dueño.
8. Si el dispositivo es irrecuperable, cada venta conocida se reconstruye; lo
   que no pueda detallarse se registra como diferencia explícita de
   recuperación, acompañada por arqueo, comprobantes digitales y conteo físico
   de inventario. No se inventan líneas de productos.

Un caso sólo puede cerrarse cuando:

- el diario fue importado o el dispositivo fue declarado irrecuperable;
- las brechas de secuencia conocidas están resueltas o documentadas;
- el efectivo fue contado y conciliado;
- los pagos digitales fueron verificados externamente;
- el inventario afectado fue contado; si no hay detalle de ventas, el conteo es
  completo;
- toda diferencia conserva incidencia, movimientos compensatorios y actor;
- el dueño aprueba el informe final.

## Garantía operativa

La recuperación nunca borra una sesión, marca sus pendientes como inexistentes
ni permite que una operación recuperada se pierda silenciosamente. Toda venta
cobrada queda como venta exacta, venta reconstruida o diferencia de
recuperación explícita. La pérdida física de la única copia puede impedir
recuperar su detalle, pero no permite cerrar el caso fingiendo que no ocurrió.

## Consecuencias

- La recuperación es más lenta que un cambio normal, pero conserva trazabilidad.
- El dueño debe conciliar caja, cobros digitales e inventario.
- El mecanismo evita tanto la doble aplicación como el descarte silencioso.
