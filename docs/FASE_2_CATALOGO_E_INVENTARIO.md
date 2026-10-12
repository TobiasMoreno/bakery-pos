# Cierre de fase 2: catálogo e inventario mínimo

- Estado: Completada y verificada localmente
- Fecha: 2026-10-12
- Alcance: catálogo, precios, importación, snapshot para POS y libro mínimo de
  inventario

## Resultado

La fase entrega un catálogo operable desde la interfaz, precios versionados,
importación CSV con previsualización, cambios masivos y un snapshot versionado
para el futuro POS. El backend incorpora además el libro mínimo de inventario
que usarán ventas, compras y producción en las fases siguientes.

## Inventario mínimo

- Las cantidades se representan como milésimos enteros en Go y como
  `NUMERIC(18,3)` en PostgreSQL; la API las serializa como strings decimales.
- Insertar un movimiento actualiza su saldo proyectado dentro de la misma
  transacción mediante un trigger de base de datos.
- Los movimientos no pueden actualizarse ni eliminarse y los saldos no pueden
  modificarse directamente. Un saldo cero puede inicializarse al crear un
  producto.
- El UUID del movimiento actúa como clave idempotente. Repetir el mismo UUID y
  payload devuelve el resultado existente; reutilizarlo con otro payload
  produce conflicto.
- Los productos cuya unidad base es `unit` rechazan cantidades fraccionarias en
  la aplicación y también en PostgreSQL.
- Cada movimiento nuevo registra actor, request ID y evento de auditoría.

## Contratos disponibles

- `GET /api/v1/inventory/balances`: saldos por producto, con búsqueda y
  paginación.
- `GET /api/v1/inventory/movements`: libro paginado, filtrable por producto y
  tipo de movimiento.
- El registro de movimientos es un caso de uso interno. No se expone una
  mutación genérica que permita alterar stock sin un origen de negocio.

## Evidencia de aceptación

Las pruebas unitarias cubren parsing decimal exacto, límite de tres decimales,
serialización sin `float`, motivos obligatorios y hash idempotente. Las pruebas
contra PostgreSQL real verifican:

1. aplicación atómica de movimiento y saldo;
2. reconstrucción del saldo desde el libro;
3. reintento sin doble efecto ni auditoría duplicada;
4. conflicto ante el mismo ID con payload distinto;
5. rechazo de fracciones para productos por unidad;
6. inmutabilidad de movimientos y protección de la proyección de saldos;
7. compatibilidad de la migración con las pruebas de catálogo existentes.

Comandos reproducibles:

```sh
make test
make lint
make test-integration
```

## Límite de la fase

Los conteos físicos, saldos iniciales operativos, ajustes manuales, compras,
lotes, mermas y devoluciones pertenecen a la fase 5. La fase 3 reutilizará este
libro para registrar la salida de stock dentro de la transacción de cada venta.
