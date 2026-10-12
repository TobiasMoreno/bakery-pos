# Bakery POS

Sistema POS integral para una panadería, desarrollado como monolito modular con
Go, React, TypeScript y PostgreSQL.

## Estado

Fase 1 implementada: fundaciones locales, autenticación y shell web. Las
funcionalidades de catálogo, ventas, caja e inventario se incorporan por fases y
todavía no están disponibles.

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
- `make test-db`: inicia PostgreSQL de pruebas, aislado en el puerto 5433 y sin
  volumen persistente.
- `make lint`: ejecuta validaciones estáticas.
- `make down`: detiene los servicios.

## Documentación

- [Especificación del MVP](./docs/ESPECIFICACION_TECNICA_PANADERIA_POS_MVP.md)
- [Cierre de fase 0](./docs/FASE_0_CIERRE_DE_DECISIONES.md)
- [ADRs](./docs/adr/README.md)
- [Plan de implementación](./docs/PLAN_IMPLEMENTACION_MVP.md)

## Seguridad local

Los valores del ejemplo son deliberadamente locales. Los secretos reales no se
guardan en Git. La integración fiscal y el despliegue productivo están fuera de
esta etapa.
