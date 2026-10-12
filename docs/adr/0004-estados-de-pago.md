# ADR-0004: Estados y verificación de pagos

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

El sistema registra medios de pago, pero no procesa ni consulta automáticamente
Mercado Pago, tarjetas o bancos. “Registrado” no equivale a “acreditado”.

## Decisión

Cada asignación de pago tiene exactamente uno de estos estados:

- `pending`: se inició o seleccionó el pago, pero todavía no existe reporte ni
  verificación suficiente. No cubre el total para completar una venta.
- `reported`: el operador o cliente declara que el pago digital fue realizado y
  puede adjuntar una referencia. Cubre operativamente la venta, pero queda
  identificado como no verificado.
- `confirmed`: el efectivo fue recibido o una persona verificó el pago en el
  canal externo. Requiere `confirmed_by`, timestamp y mecanismo de verificación.
- `rejected`: el canal externo informó o el operador comprobó que el pago no se
  acreditó. No cubre la venta.

Transiciones permitidas:

- `pending` -> `reported`, `confirmed` o `rejected`;
- `reported` -> `confirmed` o `rejected`;
- `confirmed` y `rejected` son terminales.

Un contracargo o una reversa posterior no cambia un pago confirmado: crea un
nuevo evento compensatorio y una incidencia.

Reglas:

1. Efectivo recibido se registra directamente como `confirmed`.
2. Un pago digital offline se registra como `reported`, salvo que el operador
   declare y documente que lo verificó externamente.
3. Una venta puede completarse cuando la suma de pagos `reported` y
   `confirmed` iguala el total. Debe quedar visible como “cobro sin verificar”
   mientras exista monto `reported`.
4. Pagos `pending` o `rejected` no cuentan para completar la venta.
5. Si un pago `reported` se rechaza después, no se edita ni anula la venta:
   se abre una incidencia de cobro. Un pago sustituto se registra como una nueva
   asignación.
6. Reportes separan montos pendientes, reportados, confirmados y rechazados.
7. En caja sólo ingresan pagos en efectivo confirmados.

## Consecuencias

- El sistema nunca afirma acreditación automática.
- El negocio puede continuar ante una verificación digital manual, pero el
  riesgo queda visible.
- La historia de intentos y reemplazos de pago permanece auditable.
