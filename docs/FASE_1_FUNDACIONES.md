# Cierre de Fase 1 — Fundaciones ejecutables

- Fecha: 2026-10-11
- Estado: completada
- Alcance: infraestructura local, autenticación y shell web; sin funciones de
  negocio de las fases posteriores

## Resultado

La raíz mínima del monorepo quedó ejecutable con Go, React + TypeScript + Vite,
PostgreSQL 18 y Docker Compose. Sólo se crearon los paquetes necesarios para
configuración, persistencia, autenticación y transporte HTTP.

La migración inicial crea usuarios, sesiones y auditoría. Las contraseñas usan
Argon2id; los tokens de sesión son opacos y sólo se persiste su hash. La cookie
es `HttpOnly`, `SameSite=Lax` y puede marcarse `Secure` por configuración. El
backend valida origen en operaciones mutantes, limita intentos fallidos de
login, asigna request ID y aplica capacidades en servidor.

El frontend incluye el shell responsive y accesible de acceso y sesión. No
simula catálogo, caja, ventas ni inventario.

## Criterios de aceptación verificados

1. `docker compose up -d --build --wait` inicia base, migración, API y frontend
   desde una base vacía.
2. PostgreSQL de desarrollo persiste en un volumen y PostgreSQL de pruebas usa
   un `tmpfs` independiente en el puerto 5433.
3. `/healthz` y `/readyz` responden `200`; readiness verifica PostgreSQL.
4. El seed crea o actualiza el dueño demo sin duplicarlo.
5. Login válido crea una sesión; `/api/v1/me` devuelve el usuario autenticado;
   logout revoca la sesión y el uso posterior devuelve `401`.
6. Credenciales inválidas, rate limiting, autenticación y autorización negativa
   tienen pruebas automatizadas.
7. `owner` y `seller` reciben capacidades fijas y el middleware del servidor
   rechaza capacidades ausentes.
8. El frontend pasó lint, type-check, pruebas y build. El usuario verificó el
   ingreso en navegador.
9. No existen módulos vacíos de catálogo, ventas, caja, inventario, producción,
   sincronización o reportes.

## Evidencia ejecutada

```text
Backend:  gofmt, go vet ./..., go test ./...
Frontend: npm run lint, npm run typecheck, npm test -- --run, npm run build
Compose:  docker compose config --quiet
Runtime:  migración de DB principal vacía, seed, health, readiness,
          login, me, logout y rechazo de sesión revocada
Test DB:  arranque aislado y migración 00001_foundations.sql
```

CI reproduce formato, análisis estático, tests, build, validación de Compose y
migración sobre PostgreSQL vacío en `.github/workflows/ci.yml`.

## Próximo incremento

La Fase 2 puede comenzar por la migración de categorías, productos, unidades y
precios versionados, seguida por los casos de uso y contratos HTTP. El libro
mínimo de inventario se incorpora cuando el catálogo ya tenga identidades y
unidades estables.
