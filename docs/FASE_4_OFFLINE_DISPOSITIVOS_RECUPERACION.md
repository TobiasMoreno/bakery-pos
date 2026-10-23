# Avance de fase 4 — Offline, dispositivos y recuperación

- Fecha: 2026-10-15
- Estado: en implementación; no cerrada

## Entregado

- Shell PWA instalable con manifest, icono, precache del build y fallback de
  navegación.
- IndexedDB con los stores definidos por el protocolo, prueba de escritura y
  solicitud de almacenamiento persistente.
- `checkout_intent` durable antes de mostrar el cobro; el estado `collecting`
  bloquea otra venta hasta consolidarlo.
- Consolidación local atómica del intent, operación, secuencia y hash previo.
- Autorización offline firmada con HMAC, acotada a usuario, dispositivo, sesión,
  turno, catálogo y 24 horas.
- Pagos digitales offline limitados a `reported`.
- Sincronización ordenada, detección de huecos/hash, ACK idempotente y
  conservación local del diario.
- Precios históricos de la versión preparada: la sincronización no recalcula
  contra el precio vigente.
- Recuperación forzada exclusiva del dueño: cerca la sesión anterior, conserva
  su último ACK, abre el caso y adquiere el dispositivo de reemplazo.
- Una operación del dispositivo cercado se guarda como evidencia y no aplica
  efectos de venta.
- Las diferencias de recuperación tienen resolución explícita y auditada; el
  cierre final se rechaza mientras exista alguna diferencia abierta.
- La reconstrucción manual reserva el vínculo con la operación original antes
  de aplicar la venta. Venta, pagos, caja, inventario, incidencias, auditoría y
  vínculo se confirman o revierten en una sola transacción serializable.
- Reintentos y solicitudes concurrentes no duplican efectos; cuando reaparece
  el `source_operation_id`, su ACK referencia la venta ya reconstruida.
- Cierre provisional y apertura del turno siguiente en el mismo dispositivo:
  operación, secuencia, hash, evento de turno y contexto preparado se confirman
  en una única transacción IndexedDB; el servidor los consolida en orden y
  devuelve ACK idempotente.
- Contrato OpenAPI, migración append-only y pruebas unitarias del envelope.

## Pendiente para cerrar la fase

1. Validar y endurecer el desbloqueo local seguro al reabrir la PWA sin
   conectividad.
2. Ejecutar OFF-00 a OFF-06, REC-03 a REC-08 y CASH-01 en los navegadores y
   dispositivos reales elegidos; registrar la matriz de resultados.

Hasta completar esos puntos, esta evidencia no declara la fase terminada.

## Verificación ejecutada el 2026-10-15

- Suite backend y `go vet` completos.
- Suite frontend completa, lint, type-check y build de producción.
- Migraciones `00001` a `00009` sobre PostgreSQL de pruebas vacío.
- Suite de integración completa contra PostgreSQL real, incluido CASH-01,
  reconstrucción concurrente y atómica, reintento idempotente y bloqueo del
  cierre con diferencias abiertas.

## Verificación reproducible

```text
cd backend && go test ./...
cd backend && go vet ./...
cd frontend && npm run lint
cd frontend && npm run typecheck
cd frontend && npm test -- --run
cd frontend && npm run build
docker compose --profile test up -d --wait db-test
docker compose --profile test run --rm --build --no-deps backend goose -dir /app/migrations postgres <TEST_DATABASE_URL> up
```
