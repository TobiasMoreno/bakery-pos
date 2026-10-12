# ADR-0002: Un dispositivo activo para operaciones POS

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

El negocio tendrá una sola caja física y una sola fuente activa de ventas, pero
otros dispositivos deben poder consultar datos y realizar tareas
administrativas online.

Un bloqueo global por dispositivo impediría innecesariamente catálogo, compras,
producción, inventarios, reportes y gestión de usuarios.

## Decisión

Se utilizará una sesión de dispositivo POS con versión de cercado
(`fencing_version`). Sólo una sesión puede estar en estado `active`.

La sesión POS se exige para:

- registrar una venta;
- guardar una venta offline;
- sincronizar operaciones POS;
- cerrar o abrir turnos offline en la secuencia del puesto de venta;
- ejecutar el cambio normal o la recuperación forzada del puesto.

No se exige sesión POS para:

- consultas;
- catálogo y precios;
- compras y proveedores;
- producción y recetas;
- conteos y ajustes autorizados de inventario;
- reportes, usuarios y configuración.

Los movimientos de la caja física y los cambios de turno deben respetar el
turno único vigente. Cuando se hagan desde un dispositivo distinto estando
online, incrementarán la versión del turno; el dispositivo POS deberá refrescar
esa versión antes de su siguiente operación online. No se permitirán estos
cambios remotos mientras el turno POS tenga operaciones offline pendientes,
salvo mediante el flujo de recuperación forzada.

## Adquisición y liberación normal

Para liberar una sesión se requiere conexión, cola local vacía y confirmación
del servidor. El nuevo dispositivo adquiere una versión de cercado superior y
descarga catálogo, usuario y turno vigentes.

## Recuperación forzada

Sólo el dueño puede iniciarla. La sesión anterior pasa a
`recovery_required`, queda impedida para nuevas ventas y se abre un caso de
conciliación. El nuevo dispositivo recibe una versión de cercado superior.

Una operación posterior de la sesión anterior nunca se aplica silenciosamente:
se incorpora como evidencia al caso de recuperación y se concilia según
ADR-0003.

## Consecuencias

- La exclusividad afecta a las ventas, no a toda la aplicación.
- Un dispositivo administrativo no puede convertirse en POS sin una
  adquisición explícita.
- El fencing reduce duplicados, pero no elimina la necesidad de conciliación si
  el dispositivo anterior vendió desconectado.
