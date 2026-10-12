# Pruebas críticas de aceptación

- Versión: 2.0
- Fecha: 2026-10-11

Estas pruebas complementan, no reemplazan, las 18 pruebas obligatorias de la
especificación. Los escenarios monetarios se validan contra base real y los
offline mediante navegador con IndexedDB y service worker.

## 1. Persistencia e idempotencia offline

### OFF-00 — Intent durable antes del cobro

**DADO** que IndexedDB no puede persistir un nuevo intent  
**CUANDO** el operador intenta pasar a la pantalla de pago  
**ENTONCES** el POS no habilita el cobro ni muestra instrucciones para recibir
dinero.

### OFF-01 — Consolidación local fallida después del cobro

**DADO** un intent `collecting` durable y una falla al consolidar la operación  
**CUANDO** el pago ya pudo haber sido recibido  
**ENTONCES** no se muestra “guardado localmente”, el intent queda visible como
“cobro sin consolidar” y se bloquea una nueva venta hasta reconstruirlo o
cancelarlo de forma auditada.

### OFF-02 — Reintento idéntico

**DADO** una operación ya confirmada por servidor  
**CUANDO** se reenvía el mismo `operation_id` y hash  
**ENTONCES** se obtiene el mismo ACK y existen una sola venta y un único juego
de movimientos.

### OFF-03 — ID reutilizado con payload distinto

**DADO** una operación conocida  
**CUANDO** llega el mismo ID con otro hash  
**ENTONCES** se responde conflicto, no se aplica ningún efecto y se detiene la
secuencia.

### OFF-04 — Hueco de secuencia

**DADO** que el último ACK es 20  
**CUANDO** llega la secuencia 22 sin la 21  
**ENTONCES** no se aplica la 22 y el hueco queda visible.

### OFF-05 — Precio de snapshot antiguo

**DADO** una venta offline válida con catálogo versión 10 y un precio cambiado
en versión 11  
**CUANDO** se sincroniza  
**ENTONCES** conserva el monto cobrado de versión 10, registra esa versión y no
recalcula la venta.

### OFF-06 — Autorización vencida

**DADO** un grant offline vencido  
**CUANDO** se intenta iniciar una venta nueva  
**ENTONCES** el POS la bloquea; las ventas creadas antes del vencimiento siguen
disponibles para sincronizar.

## 2. Dispositivo y recuperación forzada

### REC-01 — Administración concurrente permitida

**DADO** un dispositivo POS activo  
**CUANDO** otro dispositivo consulta reportes o registra una compra autorizada
online  
**ENTONCES** la operación administrativa está permitida y no adquiere permiso
para vender.

### REC-02 — Segunda venta rechazada

**DADO** un dispositivo POS activo  
**CUANDO** otro dispositivo intenta registrar una venta  
**ENTONCES** el servidor la rechaza por sesión POS y no crea efectos parciales.

### REC-03 — Cambio normal con pendientes

**DADO** que la cola local no está vacía  
**CUANDO** se intenta liberar normalmente la sesión POS  
**ENTONCES** el cambio queda bloqueado hasta sincronizar o iniciar recuperación
forzada.

### REC-04 — Operación del dispositivo cercado

**DADO** una recuperación forzada y una nueva versión de cercado  
**CUANDO** reaparece una venta pendiente del dispositivo anterior  
**ENTONCES** no se descarta ni se aplica automáticamente: se adjunta como
evidencia al caso de recuperación.

### REC-05 — Reconstrucción y original posterior

**DADO** una venta reconstruida con vínculo al `source_operation_id`  
**CUANDO** llega después la operación original equivalente  
**ENTONCES** se vincula al resultado existente, devuelve ACK y no duplica venta,
caja ni stock.

### REC-06 — Coincidencia ambigua

**DADO** dos ventas reconstruidas con igual total y hora aproximada  
**CUANDO** llega una operación sin referencia inequívoca  
**ENTONCES** no se vincula automáticamente y el dueño debe resolver el conflicto.

### REC-07 — Dispositivo irrecuperable

**DADO** que la única copia local fue destruida  
**CUANDO** se intenta cerrar el caso  
**ENTONCES** se exige arqueo, revisión de pagos digitales, reconstrucción de
ventas conocidas, conteo completo de inventario e incidencias por diferencias;
no se inventan líneas de venta.

### REC-08 — Cierre bloqueado

**DADO** un caso con una brecha sin resolver o sin aprobación del dueño  
**CUANDO** se intenta cerrar definitivamente el turno afectado  
**ENTONCES** el sistema lo rechaza.

## 3. Pagos

### PAY-01 — Pago pendiente

**DADO** un total de ARS 10.000 y un pago digital `pending` por ARS 10.000  
**CUANDO** se intenta completar  
**ENTONCES** la venta permanece pendiente y no afecta stock ni caja.

### PAY-02 — Pago reportado

**DADO** un pago digital reportado que cubre el total  
**CUANDO** se completa la venta  
**ENTONCES** queda `completed`, el pago permanece `reported` y la interfaz y
reportes indican “sin verificar”.

### PAY-03 — Confirmación manual externa

**DADO** un pago `reported`  
**CUANDO** un empleado verifica acreditación en el canal externo  
**ENTONCES** pasa a `confirmed` con actor, fecha y mecanismo de verificación.

### PAY-04 — Rechazo posterior

**DADO** una venta entregada con pago `reported`  
**CUANDO** el pago se marca `rejected`  
**ENTONCES** la venta no desaparece, su cobro derivado queda `issue` y se abre
incidencia por el saldo faltante.

### PAY-05 — Pago sustituto

**DADO** un pago rechazado  
**CUANDO** el cliente paga por otro medio  
**ENTONCES** se crea una asignación nueva; el intento rechazado permanece en el
historial y el estado derivado vuelve a `unverified` o `settled`.

### PAY-06 — Digital offline

**DADO** una venta offline con transferencia no comprobada  
**CUANDO** el operador la registra  
**ENTONCES** el pago sólo puede quedar `reported`, nunca `confirmed`
automáticamente.

## 4. Correcciones y devoluciones

### COR-01 — Devolución parcial

**DADO** una venta de 10 unidades sin devoluciones  
**CUANDO** se confirman 3 unidades  
**ENTONCES** la venta queda `partially_returned`, se compensan sólo 3 y la venta
original permanece intacta.

### COR-02 — Límite concurrente

**DADO** un saldo retornable de 3 unidades  
**CUANDO** dos solicitudes concurrentes intentan devolver 2 cada una  
**ENTONCES** sólo una puede reservar/confirmar si la otra excedería el saldo.

### COR-03 — Anulación después de devolución

**DADO** una venta con devolución parcial confirmada  
**CUANDO** se intenta una anulación completa  
**ENTONCES** se rechaza y sólo se permite ajustar el saldo restante.

### COR-04 — Corrección con nueva venta

**DADO** una venta con precio o producto incorrecto  
**CUANDO** se corrige  
**ENTONCES** se crea compensación y una nueva venta enlazada; no se editan
ítems, precio ni pagos originales.

### COR-05 — Reintegro digital pendiente

**DADO** una devolución digital cuyo reintegro no fue verificado  
**CUANDO** se recibe físicamente el producto  
**ENTONCES** el ajuste queda `pending_refund` y el producto en custodia no
vendible; no se informa reintegro confirmado.

### COR-06 — Aptitud de devolución

**DADO** un alimento devuelto  
**CUANDO** se marca `waste`  
**ENTONCES** se registra descarte y nunca aumenta stock vendible.

### COR-07 — Reintento de devolución

**DADO** una devolución confirmada  
**CUANDO** se reenvía la misma clave y payload  
**ENTONCES** retorna el mismo resultado sin duplicar reintegro ni inventario.

## 5. Turnos y caja

### CASH-01 — Cierre provisional ordenado

**DADO** un cierre offline y apertura posterior en el mismo dispositivo  
**CUANDO** vuelve la conexión  
**ENTONCES** se sincronizan en secuencia y sólo después el cierre pasa a
`closed`.

### CASH-02 — Cierre histórico inmutable

**DADO** un turno definitivo  
**CUANDO** aparece un ajuste tardío  
**ENTONCES** se registra en el turno vigente con referencia al histórico; no se
reescriben sus importes.

### CASH-03 — Retiro no reduce ventas

**DADO** ARS 300.000 de efectivo y un retiro de ARS 200.000  
**CUANDO** se calcula el cierre  
**ENTONCES** quedan ARS 100.000 esperados y la cifra de ventas no cambia.

## 6. Trazabilidad mínima exigida

Cada escenario mutante debe comprobar:

- actor y, cuando aplique, turno/dispositivo;
- request ID y clave idempotente;
- estado anterior y posterior;
- referencias a venta/pago/movimientos;
- ausencia de dobles efectos;
- error uniforme sin filtrar secretos;
- que el ledger original sigue inmutable.
