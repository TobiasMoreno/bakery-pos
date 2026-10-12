# Registro de decisiones de arquitectura

Este directorio contiene las decisiones que cierran la fase 0 del MVP. La
especificación funcional sigue siendo la baseline de alcance. Los ADRs refinan
sus ambigüedades sin agregar funcionalidades fuera del MVP.

## Estados

- `Propuesto`: todavía requiere aprobación.
- `Aceptado`: decisión vigente para la implementación.
- `Reemplazado`: se conserva por trazabilidad, pero otro ADR tiene precedencia.

## Decisiones aceptadas

| ADR | Decisión |
|---|---|
| [ADR-0001](./0001-monolito-modular-incremental.md) | Monolito modular incremental, sin estructura vacía |
| [ADR-0002](./0002-dispositivo-pos-activo.md) | Un dispositivo activo solamente para operaciones POS |
| [ADR-0003](./0003-sincronizacion-offline-y-recuperacion.md) | Sincronización idempotente y recuperación forzada conciliable |
| [ADR-0004](./0004-estados-de-pago.md) | Estados y verificación de pagos |
| [ADR-0005](./0005-correcciones-y-devoluciones.md) | Correcciones mediante movimientos compensatorios |
| [ADR-0006](./0006-autenticacion-y-autorizacion-offline.md) | Sesiones, permisos y autorización offline limitada |
| [ADR-0007](./0007-convenciones-de-dominio-mvp.md) | Valores iniciales para las decisiones de dominio pendientes |

Toda modificación relevante debe crear un ADR nuevo. No se reescribe una
decisión aceptada de forma que se pierda su historia.
