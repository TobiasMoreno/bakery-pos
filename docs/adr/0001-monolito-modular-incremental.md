# ADR-0001: Monolito modular incremental

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

La especificación enumera módulos lógicos de negocio. Crear desde el inicio un
árbol completo de paquetes, interfaces y adaptadores vacíos produciría
abstracciones sin casos de uso reales y aumentaría el costo de mantenimiento.

## Decisión

Se implementará un único backend Go desplegable, con una sola base PostgreSQL y
límites modulares dentro del proceso.

Los módulos se crearán cuando una fase implemente su primer caso de uso. Cada
módulo comenzará con la menor cantidad de archivos y capas necesaria. Se
separarán dominio, aplicación, transporte y persistencia sólo cuando exista una
responsabilidad concreta que lo justifique.

Reglas:

1. No crear directorios, interfaces, repositorios ni DTOs sin un consumidor.
2. No usar repositorios genéricos.
3. Una interfaz requiere al menos un límite real de sustitución o de prueba.
4. Los casos de uso que afectan varios módulos se ubican en la capa de
   aplicación y controlan una única transacción SQL.
5. Los módulos pueden compartir la transacción, pero no editar directamente las
   tablas de otro módulo fuera del caso de uso coordinador.
6. La comunicación interna será síncrona. No se incorporan microservicios,
   broker, event bus ni outbox en el MVP.
7. La lista de módulos de la especificación es un mapa de responsabilidades, no
   una obligación de crear paquetes vacíos.

## Consecuencias

- El diseño físico evolucionará junto con funcionalidades demostrables.
- Las dependencias serán explícitas y fáciles de probar.
- Un caso de uso como `ConfirmSale` podrá coordinar POS, pagos, caja,
  inventario, auditoría y sincronización dentro de la misma transacción.
- Si un módulo crece, podrá subdividirse mediante un ADR posterior.

## Alternativas descartadas

- Microservicios: complejidad operativa injustificada para una panadería y una
  sola caja.
- Arquitectura limpia completa por módulo desde el primer día: demasiados
  archivos y contratos hipotéticos.
- Acceso SQL libre desde cualquier paquete: reduce el aislamiento y vuelve
  frágiles las invariantes transaccionales.
