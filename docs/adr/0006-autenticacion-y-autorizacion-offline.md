# ADR-0006: Autenticación y autorización offline limitada

- Estado: Aceptado
- Fecha: 2026-10-11

## Contexto

El POS debe continuar durante cortes breves, pero no debe permitir el primer uso
offline de un dispositivo no preparado ni sostener indefinidamente permisos que
el servidor ya no puede comprobar.

## Decisión

Online se usarán sesiones opacas, revocables y almacenadas en servidor mediante
cookie `HttpOnly`, `Secure` y `SameSite`. Las credenciales/PIN se almacenarán
con Argon2id y salt aleatorio.

Para vender offline el servidor emitirá una autorización firmada y acotada que
incluye usuario, permisos POS, sesión de dispositivo, versión de cercado,
emisión y vencimiento. Valor inicial: 24 horas.

Reglas:

1. Sólo el dispositivo POS activo puede recibir la autorización offline.
2. El dispositivo debe haber iniciado sesión, descargado catálogo y abierto el
   turno online al menos una vez.
3. Al vencer la autorización no se crean ventas nuevas offline. Las ya cobradas
   dentro de su vigencia siguen sincronizándose.
4. Al sincronizar, el servidor valida firma, permiso que existía al emitirla,
   rango temporal y versión de dispositivo.
5. Una baja de usuario posterior no borra una venta ya cobrada bajo una
   autorización válida; se acepta y se genera una alerta de revisión si la baja
   ocurrió durante el corte.
6. Operaciones administrativas permanecen online en el MVP.
7. La reapertura offline del POS requiere desbloqueo local y una autorización
   aún vigente; no equivale a crear una nueva sesión de servidor.
8. Los intentos online se limitan y auditan. El control local es sólo una barrera
   operativa y no sustituye la validación al sincronizar.

## Permisos iniciales

Se mantienen roles fijos `seller` y `owner`. Las capacidades operativas
opcionales son `production:write` e `inventory:adjust`. No se incorpora un
editor arbitrario de roles en el MVP.

## Consecuencias

- El modo offline sirve para contingencias, no para operación indefinida.
- La revocación inmediata no puede imponerse a un dispositivo sin conexión; el
  riesgo queda acotado por tiempo, dispositivo y auditoría.
