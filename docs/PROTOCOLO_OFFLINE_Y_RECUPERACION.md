# Protocolo offline y recuperación de dispositivo

- Estado: Definitivo para el MVP
- Fecha: 2026-10-11
- ADRs relacionados: [ADR-0002](./adr/0002-dispositivo-pos-activo.md), [ADR-0003](./adr/0003-sincronizacion-offline-y-recuperacion.md), [ADR-0006](./adr/0006-autenticacion-y-autorizacion-offline.md)

## 1. Objetivos

1. Registrar ventas durante cortes breves en el dispositivo POS preparado.
2. Confirmar localmente una venta sólo después de persistirla.
3. Sincronizar exactamente una vez en términos de efectos de negocio.
4. Detectar huecos, alteraciones y reintentos.
5. Cambiar normalmente de dispositivo sólo con cola vacía.
6. Ante pérdida, impedir descartes silenciosos y exigir conciliación.
7. Permitir administración online desde otros dispositivos sin convertirlos en
   puestos de venta.

## 2. Límites de la garantía

IndexedDB no es un respaldo remoto. Si un dispositivo es destruido antes de
sincronizar y era la única copia, el detalle exacto puede ser físicamente
irrecuperable.

El MVP garantiza que:

- una operación disponible nunca se descarta por una recuperación forzada;
- sus efectos no se aplican dos veces;
- cada hueco queda abierto hasta conciliación;
- la pérdida de detalle produce una diferencia explícita y auditada, no una
  venta inventada ni un cierre aparentemente correcto.

La continuidad real también exige el procedimiento operativo: comprobantes
externos, arqueo inmediato, conteo de stock y registro de las ventas recordadas.

## 3. Preparación del dispositivo

Antes de poder vender offline, el dispositivo debe:

1. estar registrado y haber adquirido la sesión POS online;
2. autenticar al empleado y recibir autorización offline vigente;
3. descargar un snapshot completo de catálogo, precios y versión;
4. conocer el turno abierto y su versión;
5. instalar el shell PWA y verificar disponibilidad offline;
6. obtener almacenamiento persistente cuando el navegador lo permita;
7. completar una prueba de escritura/lectura IndexedDB.

Un dispositivo nunca preparado muestra sólo una pantalla de contingencia y no
permite iniciar ventas offline.

## 4. Diario local

Tablas lógicas de IndexedDB:

- `catalog_snapshots`;
- `device_session`;
- `offline_authorization`;
- `shift_events`;
- `checkout_intents`;
- `operations`;
- `sync_attempts`;
- `recovery_exports`.

Cada operación contiene el envelope definido por ADR-0003. El payload se
serializa de forma canónica antes de calcular hashes.

Antes de mostrar la pantalla que instruye cobrar, el POS:

1. crea un `checkout_intent` con ID, carrito congelado, total, turno y estado
   `ready_for_payment`;
2. comprueba que la transacción IndexedDB fue confirmada;
3. al pulsar “iniciar cobro”, cambia el intent a `collecting` y confirma otra
   transacción;
4. recién entonces permite recibir efectivo o solicitar el pago digital.

Si no puede persistir esos estados, el POS no debe iniciar el cobro.

Después de recibir o reportar el pago, la confirmación local ejecuta una sola
transacción IndexedDB que:

1. valida autorización, sesión, turno y versión local;
2. asigna UUID y siguiente secuencia;
3. congela snapshots de ítems y pagos;
4. calcula `previous_hash` y `payload_hash`;
5. inserta la operación `pending`;
6. marca el `checkout_intent` como `consolidated` y lo enlaza a la operación;
7. actualiza el último número local y hash de sesión;
8. confirma la transacción;
9. recién entonces muestra “guardado localmente” y el número local.

Si la consolidación falla, la interfaz no declara la venta guardada. El intent
permanece `collecting`, se muestra “cobro sin consolidar” y no se permite iniciar
otra venta hasta confirmar si hubo pago y reconstruir o cancelar el intento de
forma auditada. Al reabrir la aplicación, todo intent `collecting` se presenta
antes de habilitar el POS.

## 5. Sincronización normal

Disparadores:

- apertura de la aplicación;
- recuperación comprobada de conectividad con la API;
- acción manual;
- Background Sync como optimización, nunca como único mecanismo.

Flujo:

1. Seleccionar operaciones `pending` o `failed` recuperables en orden.
2. Enviar lotes acotados con sesión, versión y último ACK conocido.
3. El servidor autentica, valida envelope, autorización y continuidad de
   secuencia/hash.
4. Para cada operación:
   - si ID/hash ya existe, devolver el ACK original;
   - si ID o secuencia colisiona con otro hash, responder conflicto;
   - si es nueva, aplicar el caso de uso completo en una transacción SQL;
   - guardar el resultado idempotente antes del commit.
5. Responder un ACK por operación.
6. IndexedDB marca `acknowledged` en otra transacción y conserva el registro.
7. Ante error recuperable aplicar backoff; ante conflicto detener la secuencia.

La indisponibilidad de stock nunca rechaza una venta: produce saldo negativo e
incidencia. Un catálogo antiguo tampoco recalcula lo ya cobrado.

## 6. Cambio normal de dispositivo

1. Sincronizar hasta obtener cero pendientes, conflictos y fallidos.
2. Cerrar o entregar el turno según corresponda.
3. Liberar la sesión POS en servidor.
4. Adquirirla desde el nuevo dispositivo con una nueva versión de cercado.
5. Descargar snapshot y turno vigente.
6. Ejecutar prueba offline antes de operar.

Sin conexión no existe cambio normal.

## 7. Recuperación forzada

### 7.1 Apertura

El dueño informa motivo y confirma que no puede completar el cambio normal. El
servidor:

1. congela la sesión anterior como `recovery_required`;
2. guarda su último ACK, secuencia y hash conocidos;
3. abre un caso de recuperación;
4. exige conteo del efectivo físico y revisión inicial de pagos digitales;
5. asigna al nuevo dispositivo una versión de cercado superior;
6. permite abrir un nuevo turno vinculado al caso.

### 7.2 Operación posterior del dispositivo anterior

Si reaparece, la aplicación detecta el cercado vencido y entra en modo de sólo
recuperación. Permite inspeccionar pendientes, generar exportación e intentar
sincronizarlos contra el caso. No permite nuevas ventas ni modificar payloads.

### 7.3 Reconstrucción manual

El dueño registra cada venta conocida con:

- clave idempotente propia;
- fecha/hora aproximada o exacta;
- referencia local/externa disponible;
- ítems conocidos, total y pagos;
- grado de certeza y evidencia;
- operador probable;
- disposición de inventario que corresponda.

Una venta exacta se contabiliza por el pipeline normal con origen
`device_recovery`. Si sólo se conoce un total, se registra una diferencia de
recuperación; no se crea un producto ficticio ni se atribuyen ítems arbitrarios.

### 7.4 Reencuentro con operaciones originales

- Coincidencia exacta por `operation_id`: vincular y devolver el resultado ya
  contabilizado.
- Coincidencia inequívoca por referencia, hora, total y pago: proponer vínculo y
  exigir confirmación del dueño.
- Coincidencia ambigua: mantener conflicto abierto.
- Diferencia frente a reconstrucción: conservar ambos como evidencia y aplicar
  solamente movimientos delta autorizados.

### 7.5 Cierre

El caso produce un informe con:

- rango de secuencias reconocido y faltante;
- ventas originales, reconstruidas y diferencias sin detalle;
- efectivo esperado y contado;
- pagos digitales reportados, confirmados y rechazados;
- conteo y ajustes de inventario;
- incidencias abiertas;
- actor que resolvió cada correspondencia;
- aprobación final del dueño.

No se habilita el cierre definitivo del turno afectado hasta cumplir todos los
controles definidos en ADR-0003.

## 8. Seguridad y privacidad

- Los hashes de payload detectan cambios accidentales; la autenticidad proviene
  de sesión, autorización firmada y TLS cuando hay red.
- IndexedDB no almacena contraseñas ni secretos de servidor.
- Los payloads no contienen datos bancarios sensibles.
- Exportaciones de recuperación contienen sólo datos operativos necesarios y
  deben eliminarse de forma controlada después de importarlas.
- Logs técnicos no incluyen PIN, cookie, token ni payload completo de pago.

## 9. Observabilidad mínima

Métricas o contadores operativos:

- pendientes por sesión y antigüedad máxima;
- último ACK y último hash;
- conflictos por tipo;
- recuperaciones abiertas;
- ventas reconstruidas y diferencias sin detalle;
- fallos de persistencia y cuota local.
- intents de cobro `collecting` sin consolidar.

La interfaz muestra siempre uno de estos estados: `Sincronizado`, `Sin conexión
— N pendientes`, `Sincronizando`, `Conflicto — requiere revisión` o
`Recuperación en curso`.
