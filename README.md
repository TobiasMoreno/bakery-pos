# Bakery POS

Sistema POS integral para una panadería, desarrollado como monolito modular con
Go, React, TypeScript y PostgreSQL.

## Estado

Fase 4 en implementación: además del POS online se incorporaron PWA instalable,
intents de cobro y diario durable en IndexedDB, autorización offline de 24 horas,
cola ordenada con hashes y ACK idempotente, y apertura de recuperación forzada.
El cierre/apertura de turnos offline y la conciliación final asistida siguen
pendientes antes de declarar cerrada la fase.

## Requisitos

- Docker Desktop con Compose v2.
- Opcional para desarrollo nativo: Go 1.26+ y Node.js 22.13+.

## Inicio rápido

1. Copiar `.env.example` a `.env` y mantener sus valores sólo para desarrollo.
2. Ejecutar `make dev` o `docker compose up --build`.
3. En otra terminal, ejecutar `make seed`.
4. Abrir `http://localhost:5173`.

El seed local crea dos usuarios para probar permisos diferentes:

- `owner` / `change-me-local`: rol interno `owner`, con administración.
- `employee` / `employee-local`: rol interno `seller`, empleado vendedor sin
  edición de precios ni administración.

Las credenciales se pueden reemplazar mediante las variables `DEMO_*` de
`.env.example`. Nunca reutilizarlas fuera del entorno local.

## Comandos

- `make dev`: compila e inicia base, API y frontend.
- `make migrate`: aplica migraciones pendientes.
- `make seed`: crea o actualiza los dos usuarios demo.
- `make test`: ejecuta pruebas de backend y frontend.
- `make test-integration`: migra PostgreSQL de pruebas y ejecuta en serie las
  pruebas de repositorios y constraints reales.
- `make test-db`: inicia PostgreSQL de pruebas, aislado en el puerto 5433 y sin
  volumen persistente.
- `make lint`: ejecuta validaciones estáticas.
- `make down`: detiene los servicios.

## Documentación

- [Especificación del MVP](./docs/ESPECIFICACION_TECNICA_PANADERIA_POS_MVP.md)
- [Cierre de fase 0](./docs/FASE_0_CIERRE_DE_DECISIONES.md)
- [Evidencia de fase 1](./docs/FASE_1_FUNDACIONES.md)
- [Evidencia de fase 2](./docs/FASE_2_CATALOGO_E_INVENTARIO.md)
- [Evidencia de fase 3](./docs/FASE_3_POS_CAJA_ONLINE.md)
- [Avance de fase 4](./docs/FASE_4_OFFLINE_DISPOSITIVOS_RECUPERACION.md)
- [ADRs](./docs/adr/README.md)
- [Plan de implementación](./docs/PLAN_IMPLEMENTACION_MVP.md)

## Seguridad local

Los valores del ejemplo son deliberadamente locales. Los secretos reales no se
guardan en Git. La integración fiscal y el despliegue productivo están fuera de
esta etapa.
