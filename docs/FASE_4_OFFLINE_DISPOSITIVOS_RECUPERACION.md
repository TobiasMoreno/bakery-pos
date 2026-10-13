# Avance de fase 4 — Offline, dispositivos y recuperación

- Fecha: 2026-10-12
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
- Contrato OpenAPI, migración append-only y pruebas unitarias del envelope.

## Pendiente para cerrar la fase

1. Modelar cierre provisional y apertura offline como operaciones del mismo
   diario, y consolidarlas en secuencia.
2. Completar la UI y endpoints de conciliación: reconstrucción manual, vínculo
   inequívoco con el original, diferencias sin detalle y aprobación final.
3. Incorporar desbloqueo local seguro al reabrir la PWA sin conectividad.
4. Ejecutar OFF-00 a OFF-06, REC-03 a REC-08 y CASH-01 en los navegadores y
   dispositivos reales elegidos; registrar la matriz de resultados.

Hasta completar esos puntos, esta evidencia no declara la fase terminada.

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
