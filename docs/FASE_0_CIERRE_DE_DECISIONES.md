# Cierre de fase 0

- Estado: Completada documentalmente
- Fecha: 2026-10-11
- Baseline: `ESPECIFICACION_TECNICA_PANADERIA_POS_MVP.md` v1.0

## Resultado

La arquitectura general queda aprobada como monolito modular incremental. Se
cerraron las ambigüedades necesarias para comenzar las fundaciones sin
implementar todavía funcionalidades de negocio.

## Decisiones principales

1. La exclusividad de dispositivo se aplica al puesto POS y no bloquea consultas
   o administración online desde otros equipos.
2. La sincronización usa UUID estable, secuencia, hash encadenado, versión de
   cercado y ACK persistido.
3. Una recuperación forzada abre un caso obligatorio; nunca elimina pendientes
   ni acepta silenciosamente operaciones de una sesión cercada.
4. La conciliación es idempotente y vincula ventas reconstruidas con operaciones
   originales si reaparecen.
5. La pérdida física de la única copia puede impedir recuperar detalle. El caso
   sólo puede cerrarse con arqueo, revisión digital, conteo de inventario y
   diferencias explícitas.
6. Los pagos distinguen `pending`, `reported`, `confirmed` y `rejected`.
7. Un pago digital no se considera acreditado sin verificación externa manual.
8. Anulaciones y devoluciones crean movimientos compensatorios; las correcciones
   pueden vincular una nueva venta sin editar la original.
9. Los módulos físicos se crean al implementar casos de uso, no por anticipado.
10. Las decisiones menores pendientes adoptan los valores de ADR-0007.

## Documentos normativos

- [Índice de ADRs](./adr/README.md)
- [Máquinas de estados](./MAQUINAS_DE_ESTADO_MVP.md)
- [Protocolo offline y recuperación](./PROTOCOLO_OFFLINE_Y_RECUPERACION.md)
- [Plan de implementación](./PLAN_IMPLEMENTACION_MVP.md)
- [Pruebas críticas](./PRUEBAS_CRITICAS_MVP.md)

## Precedencia

La especificación define alcance y comportamiento funcional. Estos documentos
refinan decisiones técnicas y casos ambiguos. Si aparece una contradicción real,
no se elige implícitamente: se crea un ADR que identifique la cláusula afectada y
la decisión debe ser aprobada antes de implementar.

## Validaciones operativas que permanecen abiertas

No bloquean las fundaciones, pero deben cerrarse antes del piloto:

- ubicación y corte comercial definitivo;
- navegadores y dispositivos reales;
- reglas sanitarias de aptitud, conservación y reutilización;
- catálogo, empleados, horarios, proveedores y recetas reales;
- parámetros de hash medidos en el hardware objetivo;
- procedimiento manual y responsables de una recuperación forzada.

## Próxima fase

Crear `AGENTS.md` y las fundaciones mínimas de fase 1. No deben crearse paquetes
vacíos para módulos futuros.
