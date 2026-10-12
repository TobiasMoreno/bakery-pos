# Convenciones del proyecto Bakery POS

## Alcance

Este repositorio implementa exclusivamente el MVP definido en
`docs/ESPECIFICACION_TECNICA_PANADERIA_POS_MVP.md` y refinado por los ADRs de
`docs/adr/`. Antes de cambiar una regla de negocio, leer la especificación, el
ADR aplicable y las máquinas de estados.

No agregar facturación fiscal, cobro automático, múltiples cajas, sucursales ni
otras funcionalidades fuera de la baseline sin una decisión aprobada.

## Idioma y nombres

- Código, nombres de tablas, endpoints y símbolos: inglés.
- Interfaz, mensajes para usuarios y documentación funcional: español.
- Errores API: códigos estables en inglés y mensajes seguros en español.

## Arquitectura

- Backend Go como monolito modular y una sola base PostgreSQL.
- Crear módulos, interfaces y abstracciones únicamente cuando un caso de uso
  real los necesite.
- No crear repositorios genéricos ni paquetes vacíos.
- Los casos de uso transversales controlan una única transacción SQL.
- No introducir microservicios, brokers, event bus u outbox en el MVP.
- Mantener transporte, aplicación, dominio y persistencia separados sólo cuando
  exista una responsabilidad concreta.

## Backend

- Usar `context.Context` en toda operación con I/O.
- Propagar errores de dominio tipados y convertirlos al formato API sólo en el
  borde HTTP.
- Respuesta de error uniforme: `{ code, message, details, request_id }`.
- Todas las mutaciones deben tener actor, request ID y auditoría cuando aplique.
- Usar `log/slog`; nunca registrar credenciales, cookies, tokens o payloads
  sensibles.
- Interfaces sólo cuando exista más de una implementación real o un límite de
  prueba valioso.
- Formatear con `gofmt`; validar con `go test ./...` y `go vet ./...`.

## Persistencia

- Migraciones SQL versionadas y append-only. No reescribir una migración ya
  compartida o aplicada.
- PostgreSQL es la fuente de verdad central.
- Dinero en centavos `BIGINT`/`int64`; nunca `float`.
- Cantidades persistidas con precisión exacta máxima de tres decimales; nunca
  `float` para decisiones de inventario.
- Fechas en UTC con `TIMESTAMPTZ`; conversión a zona comercial en los bordes.
- Ventas, pagos, caja, movimientos de inventario y auditoría no se borran ni se
  editan destructivamente.
- Toda actualización de saldo requiere su movimiento correspondiente.
- Constraints e idempotencia deben respaldar las invariantes, no sólo el código.

## Frontend

- React + TypeScript estricto + Vite.
- Mobile-first, táctil y accesible por teclado.
- Validar entradas en cliente por experiencia, sin sustituir validación del
  servidor.
- Estado local offline pertenece a IndexedDB; no usar el caché HTTP como cola.
- No declarar una venta guardada antes de confirmar la transacción local.
- Mantener cálculos monetarios en enteros.
- Evitar dependencias de UI pesadas; construir componentes cuando haya uso real.

## Seguridad

- Secretos únicamente por variables de entorno; `.env` no se versiona.
- Sesiones mediante cookies seguras y permisos validados en servidor.
- Contraseñas/PIN con Argon2id y salt aleatorio.
- No confiar en roles, importes, timestamps ni estados enviados por el cliente.
- Operaciones mutantes deben ser idempotentes cuando puedan reintentarse.

## Pruebas

- Reglas de dinero, permisos, estados e idempotencia requieren pruebas unitarias.
- Repositorios y transacciones críticas se prueban contra PostgreSQL real.
- Frontend: Vitest + Testing Library; flujos críticos: Playwright cuando se
  introduzcan en su fase.
- Un bug corregido debe incorporar una prueba que hubiese fallado antes.
- Ejecutar primero el chequeo más estrecho y luego la suite de la capa afectada.

## Documentación y cambios

- Actualizar OpenAPI junto con handlers y cliente.
- Una decisión que cambie un ADR aceptado requiere un ADR nuevo que lo reemplace.
- Al entregar, informar archivos modificados, comandos ejecutados y riesgos
  pendientes.
- Preservar cambios ajenos y no limpiar el worktree para simplificar una tarea.

## Comandos canónicos

- `make dev`: entorno local completo.
- `make migrate`: aplicar migraciones.
- `make seed`: crear fixtures de desarrollo.
- `make test`: pruebas backend y frontend.
- `make lint`: validaciones estáticas.
- `make down`: detener el entorno local.
