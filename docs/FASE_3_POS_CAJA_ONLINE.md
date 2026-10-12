# Evidencia de fase 3 — POS, pagos y caja online

- Fecha: 2026-10-12
- Estado: implementada y verificada localmente

## Incremento entregado

- Adquisición y liberación normal de una única sesión POS activa, con
  `fencing_version` persistida.
- Turno único de caja con apertura, versión optimista, ingresos, gastos,
  retiros, arqueo y cierre definitivo.
- Carrito táctil por unidad y gramos, cálculo monetario entero y redondeo
  `half-up` por línea.
- Venta online idempotente con snapshot de producto/precio y una única
  transacción para venta, ítems, pagos, caja, inventario, incidencias y
  auditoría.
- Pagos simples o combinados. Efectivo confirmado con recibido y vuelto;
  pagos digitales reportados o confirmados con verificación manual explícita.
- Stock negativo permitido con una incidencia abierta, sin impedir la venta.
- Historial de ventas y comprobante interno marcado como no fiscal.
- Contrato OpenAPI y cliente TypeScript actualizados junto con los handlers.

## Invariantes cubiertas

- La misma `operation_id` y el mismo payload devuelven la venta original; el
  mismo ID con otro payload produce conflicto.
- Sólo la sesión POS activa y la versión de cercado vigente pueden vender.
- El turno debe estar abierto y con la versión esperada.
- Pagos `pending` o `rejected` no completan una venta. La suma de pagos
  `reported` y `confirmed` debe coincidir exactamente con el total.
- En caja entra únicamente el neto asignado a efectivo, no el importe recibido
  antes del vuelto.
- El efectivo esperado se deriva de apertura + ventas en efectivo + ingresos −
  gastos − retiros − reintegros.
- Ventas, ítems, movimientos de caja e inventario no se eliminan. Un turno
  cerrado no puede reabrirse ni reescribirse.

## Evidencia automatizada

La prueba PostgreSQL de `internal/pos` cubre en una base real:

- 350 g a ARS 9.500/kg = ARS 3.325 y salida exacta de 350 g;
- venta sobre saldo cero con incidencia de stock negativo;
- pago mixto con efectivo neto, recibido, vuelto y transferencia reportada;
- rechazo de un segundo dispositivo POS;
- reintento idempotente y conflicto por payload distinto;
- fórmula de caja con gasto y retiro;
- cierre con esperado, contado y diferencia;
- rechazo SQL del borrado de una venta.

Comandos de verificación:

```text
cd backend && go test ./...
cd backend && go vet ./...
cd backend && TEST_DATABASE_URL=... go test -count=1 -tags=integration ./internal/pos
cd frontend && npx vitest run
cd frontend && npm run lint
cd frontend && npm run typecheck
cd frontend && npm run build
npx --yes @redocly/cli lint api/openapi.yaml --skip-rule operation-summary --skip-rule security-defined --skip-rule info-license
```

## Límite consciente

Esta fase funciona conectada. La persistencia durable previa al cobro,
IndexedDB, secuencias offline, service worker, recuperación forzada y
conciliación pertenecen a la fase 4. Hasta incorporar ese protocolo, la
interfaz no afirma que una venta quedó guardada si la API no confirmó la
transacción.
