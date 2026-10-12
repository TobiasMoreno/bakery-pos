# ADR-0005: Correcciones y devoluciones mediante compensaciones

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

Las ventas cobradas, sus ítems, pagos y movimientos no deben editarse ni
eliminarse. Aun así, el negocio necesita anular ventas completas, devolver
parcialmente y corregir una operación equivocada.

## Decisión

Se modelarán ajustes vinculados a la venta original:

- `void`: anulación completa. Sólo se confirma si compensa todo el saldo no
  devuelto de la venta.
- `return`: devolución total o parcial de cantidades e importes.
- `correction`: relación auditada entre una anulación/devolución y, cuando
  corresponda, una nueva venta de reemplazo.

Toda operación requiere actor, motivo, timestamps y referencias a la venta y a
los movimientos compensatorios.

Reglas:

1. La suma de ajustes confirmados y pendientes reservados no puede superar las
   cantidades ni el importe neto de la venta original.
2. La validación se realiza con bloqueo transaccional de la venta original.
3. Una venta con devolución confirmada no puede anularse completa; debe
   devolverse únicamente su saldo restante.
4. La disposición se decide por ítem: `restock`, `waste` o, cuando corresponda,
   `quarantine`.
5. Un alimento no vuelve a stock vendible sin decisión explícita de aptitud.
6. Las devoluciones digitales sólo se confirman después de verificar el
   reintegro por el canal externo.
7. Mientras un reintegro digital está pendiente, el producto recibido queda en
   custodia no vendible. La confirmación lo mueve a stock o descarte; el rechazo
   compensa la custodia según la resolución física.
8. Una corrección crea una nueva venta con sus propios precios y pagos y la
   enlaza al ajuste; no reemplaza snapshots históricos.
9. Los reintentos utilizan una clave idempotente y devuelven el mismo resultado.

## Consecuencias

- El historial explica qué ocurrió y cómo fue compensado.
- Caja e inventario pueden tener movimientos diferentes según método de
  reintegro y aptitud del producto.
- Las correcciones no requieren permisos superiores a los establecidos en la
  especificación, pero siempre requieren motivo.
