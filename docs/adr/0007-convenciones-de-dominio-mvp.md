# ADR-0007: Convenciones iniciales de dominio del MVP

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

La especificación deja varias decisiones menores abiertas. Deben tener un valor
inicial para evitar implementaciones incompatibles.

## Decisión

### Tiempo y día comercial

- Persistencia en UTC con `TIMESTAMPTZ`.
- Zona comercial inicial: `America/Argentina/Cordoba`.
- Corte inicial del día comercial: 00:00 local.
- La zona y el corte son configuración del negocio, no del dispositivo.

### Identidad operativa

- `actor_id` es obligatorio en toda mutación.
- `shift_id` es obligatorio para ventas y movimientos de caja, y nullable para
  administración sin turno.
- Existe un solo turno de caja operativo a la vez.

### Dinero, cantidades y redondeo

- Dinero en centavos enteros `BIGINT`/`int64`.
- Cantidades en unidad base con escala máxima de tres decimales.
- Artículos por unidad requieren cantidad entera.
- Venta al peso: multiplicación exacta y redondeo decimal `half-up` por línea;
  el total es la suma de líneas redondeadas.
- No se usa `float` para dinero ni cantidades persistidas.

### Costos

- Política inicial: último costo conocido al momento de la operación.
- El costo se guarda como snapshot en producción.
- Costo desconocido es distinto de cero y produce una marca de información
  incompleta.

### Compras

- Recepción completa y única en el MVP; no hay recepción parcial.
- Una compra recibida es inmutable. Una corrección crea movimientos
  compensatorios.
- Estado de pago de compra no modifica inventario.

### Lotes e inventario negativo

- Salida FEFO para productos configurados por lote.
- Lotes y saldos son proyecciones del libro de movimientos, no fuentes de verdad
  editables.
- Si se agotan lotes, la venta continúa con déficit no asignado e incidencia.
- Un conteo físico corrige mediante movimiento, nunca sobrescribiendo historia.

### Importación CSV

- Código de barras es único cuando existe.
- Sin código, los candidatos duplicados se detectan por nombre normalizado,
  categoría, tipo y modo de venta.
- Toda ambigüedad aparece en preview; el commit es atómico y no omite filas
  silenciosamente.

### Precios desactualizados offline

- Una venta válida conserva el precio del snapshot y la versión de catálogo
  usada, aunque exista una versión nueva al sincronizar.
- La diferencia se audita; no se recalcula el total cobrado.

## Consecuencias

Estas reglas son valores iniciales aprobados. Cambiarlas requiere un ADR y
pruebas de migración o compatibilidad histórica.
