# Plan de implementación del MVP

- Versión: 2.1
- Fecha: 2026-10-12
- Estado: Fase 4 en implementación; núcleo de venta offline y sync disponible

## Principios de ejecución

- Cada fase produce un incremento demostrable y verificable.
- Sólo se crean módulos y abstracciones requeridos por la fase actual.
- Las migraciones preceden al código que depende de ellas.
- Los contratos OpenAPI cambian junto con handlers, cliente y pruebas.
- Reglas monetarias, de inventario e idempotencia se prueban en backend y
  frontend cuando exista cálculo duplicado.
- Las operaciones críticas se prueban con PostgreSQL real.
- No se incorpora alcance fiscal, múltiples cajas, sucursales ni integraciones
  automáticas de pagos.

## Fase 0 — Decisiones y contratos de comportamiento

### Entregables

- ADRs aceptados.
- Máquinas de estados de ventas, pagos, ajustes y turnos.
- Protocolo offline y recuperación forzada.
- Plan y pruebas críticas actualizados.

### Criterios de aceptación

- Los cinco ajustes solicitados tienen una decisión explícita y verificable.
- Las demás ambigüedades tienen valores iniciales compatibles con la baseline.
- La recuperación distingue garantía del sistema y límite físico de perder la
  única copia local.
- No se creó código ni scaffolding de módulos.

Estado: completada documentalmente.

## Fase 1 — Fundaciones ejecutables

### Alcance

- `AGENTS.md` y README operativo.
- Raíz mínima del monorepo.
- Compose con PostgreSQL, backend y frontend.
- Configuración y `.env.example` sin secretos.
- Migraciones y base de prueba separada.
- Health/readiness.
- Sesiones online, usuarios, roles fijos y capacidades.
- Contrato común de errores y request ID.
- CI de compilación, lint, migraciones y tests.

### Criterios de aceptación

1. Un checkout limpio inicia el entorno con comandos documentados.
2. Migraciones suben sobre base vacía y pueden verificarse en CI.
3. Backend y frontend informan salud sin depender de fixtures manuales.
4. Login válido crea sesión segura; credenciales inválidas y rate limit se
   prueban.
5. Permisos se aplican en servidor, no sólo en navegación.
6. `seller` no administra precios/usuarios; `owner` sí.
7. No existen paquetes vacíos para módulos de fases futuras.

Estado: completada y verificada localmente el 2026-10-11. La evidencia se
encuentra en `FASE_1_FUNDACIONES.md`.

## Fase 2 — Catálogo, precios y libro mínimo de inventario

### Alcance

- Categorías, productos, unidades y códigos.
- Precios versionados y cambios masivos con preview.
- Importación CSV con preview y commit atómico.
- Libro inmutable de movimientos y saldo proyectado mínimo.
- Snapshot versionado para POS.

### Criterios de aceptación

1. Todo producto puede buscarse y seleccionarse manualmente.
2. Código de barras es único cuando existe.
3. CSV inválido o ambiguo muestra errores y no importa parcialmente.
4. Cambios masivos muestran antes/después y registran actor.
5. Un precio posterior no altera snapshots históricos de prueba.
6. El saldo se puede reconstruir desde movimientos.
7. Cantidades unitarias no aceptan fracciones.

Estado: completada y verificada localmente el 2026-10-12. La evidencia se
encuentra en `FASE_2_CATALOGO_E_INVENTARIO.md`.

## Fase 3 — POS, pagos y caja online

### Alcance

- Carrito por unidad/peso.
- Redondeo y snapshots.
- Pagos simples/combinados con estados del ADR-0004.
- Venta idempotente y transaccional.
- Apertura, movimientos, retiros, arqueo y cierre online.
- Stock negativo permitido con incidencia.
- Historial y comprobante interno.

### Criterios de aceptación

1. Venta, ítems, pagos, caja, stock, incidencia, auditoría y resultado
   idempotente confirman o revierten juntos.
2. 350 g a ARS 9.500/kg totalizan ARS 3.325 y descuentan 350 g.
3. Un pago `pending` no completa; uno `reported` deja alerta visible; sólo uno
   `confirmed` se informa acreditado.
4. Pago mixto separa efectivo neto, recibido y vuelto.
5. La fórmula de efectivo esperado reproduce los casos obligatorios.
6. Venta sobre stock disponible se permite y abre incidencia.
7. Sólo la sesión POS activa registra ventas; dispositivos administrativos
   continúan funcionando.

Estado: completada y verificada localmente el 2026-10-12. La evidencia se
encuentra en `FASE_3_POS_CAJA_ONLINE.md`.

## Fase 4 — Offline, dispositivos y recuperación

### Alcance

- PWA instalada y precache.
- IndexedDB transaccional, intent durable previo al cobro y autorización offline
  limitada.
- Cola ordenada, hashes, ACK e idempotencia.
- Adquisición/liberación normal del puesto.
- Cierre/apertura local de turnos.
- Recuperación forzada, importación de diario y conciliación.

### Criterios de aceptación

1. El POS no habilita el cobro sin intent durable; si falla la consolidación
   posterior, conserva un cobro incierto y bloquea nuevas ventas.
2. Reintentar una operación devuelve el mismo ACK sin duplicar efectos.
3. Un hash o secuencia conflictiva detiene el lote.
4. No se cambia normalmente de dispositivo con pendientes.
5. El dispositivo cercado sólo puede aportar evidencia al caso de recuperación.
6. Una venta reconstruida y su original posterior se aplican una sola vez.
7. Una brecha no detallable permanece explícita y bloquea el cierre hasta la
   conciliación requerida.
8. Cierre y apertura offline del mismo dispositivo se consolidan en orden.
9. Las pruebas se ejecutan en los navegadores/dispositivos reales elegidos.

Estado: en implementación desde el 2026-10-12. El avance y los controles aún
pendientes están documentados en `FASE_4_OFFLINE_DISPOSITIVOS_RECUPERACION.md`.

## Fase 5 — Inventario, compras, lotes y correcciones

### Alcance

- Conteos, saldos iniciales, ajustes e incidencias.
- Compras, proveedores, recepción única e ingreso rápido.
- Lotes selectivos y FEFO.
- Mermas y descartes.
- Devoluciones, anulaciones y correcciones vinculadas.
- Custodia de devoluciones con reintegro pendiente.

### Criterios de aceptación

1. Una recepción incrementa stock exactamente una vez.
2. Conteo inicial incluye ceros y respeta la hora de corte.
3. Correcciones generan compensaciones y no editan venta ni movimiento original.
4. Devoluciones concurrentes no superan cantidades o importes disponibles.
5. Reintegro digital pendiente no se presenta como confirmado.
6. Producto no apto no vuelve al stock vendible.
7. Anulación completa no es posible después de una devolución parcial.
8. Déficit de lotes no impide vender y genera incidencia no asignada.

## Fase 6 — Recetas, producción y sobrantes

### Alcance

- Recetas versionadas exactas/aproximadas.
- Producción y consumos confirmados.
- Último costo conocido y costo faltante explícito.
- Sobrantes conservados, descartados o reutilizados.

### Criterios de aceptación

1. Producción aumenta terminado y descuenta consumos en una transacción.
2. Consumos sugeridos de receta aproximada son editables antes de confirmar.
3. Producción sin datos suficientes abre incidencia; no inventa consumos.
4. Cambiar una receta no altera lotes históricos.
5. Costo faltante nunca se informa como cero.
6. Reutilización consume origen y crea destino con trazabilidad.

## Fase 7 — Reportes, puesta en marcha y endurecimiento

### Alcance

- Ventas diarias, pagos por estado/medio y cierres.
- Stock, incidencias, producción, desperdicio y costos estimados.
- Asistente de configuración e importación real.
- Backup/restauración local ensayada.
- Runbooks de contingencia, recuperación y lanzamiento.
- Pruebas E2E y ensayo con datos/dispositivos reales.

### Criterios de aceptación

1. Reportes se reconcilian con libros de movimientos y respetan día comercial.
2. Pagos reportados no se mezclan con confirmados.
3. No se usa “ganancia neta” para costos parciales.
4. Backup restaurado reproduce ventas, caja, stock, auditoría y sync.
5. Pasan las pruebas obligatorias de la especificación y las pruebas críticas
   adicionales de fase 0.
6. El dueño aprueba catálogo, flujos, reglas sanitarias y plan de contingencia.

## Puertas de calidad comunes

Una fase no se considera terminada si falta alguno de estos controles aplicable:

- compilación, format, lint y type-check;
- tests unitarios de reglas;
- tests de integración de transacciones y constraints;
- pruebas de autorización negativa;
- migraciones sobre base vacía;
- contrato OpenAPI actualizado;
- documentación operativa y ADR si cambió una decisión;
- evidencia de aceptación reproducible.
