# La API

Base: `/api/v1`. Todo JSON salvo el bundle, que es texto plano. Los errores tienen siempre la
misma forma:

```json
{ "error": "sobre inválido", "detalle": "falta el campo obligatorio `run_id`", "avisos": [] }
```

## Autenticación

Dos tokens, en `Authorization: Bearer`:

| Token | Rutas | Variable |
|---|---|---|
| emisor | `POST /corridas`, `POST /corridas/validar`, `DELETE /pipelines/...` | `LC_TOKEN_EMISOR` |
| lector | todo lo demás | `LC_TOKEN_LECTOR` |
| *(ninguno)* | `/contrato`, `/check`, `/health` | — |

Que se filtre el token de un pipeline no abre la lectura del panel. Si un token está vacío, esas
rutas quedan abiertas y el arranque lo dice con un `warn`.

## Ingesta

### `POST /corridas` → `202`

El sobre completo. Contrato en [`../INTEGRACION.md`](../INTEGRACION.md).

```json
{ "run_id": "20260831-061149-76d", "proyecto": "leads-peak-shaving",
  "pipeline": "etl_contratos", "status": "ok", "avisos": [] }
```

Idempotente por `run_id`: reenviar la misma corrida la pisa, no la duplica. Si vuelve con otro
`status`, el resumen anterior se borra.

Códigos: `202` aceptada · `400` sobre inválido · `401` sin token · `413` supera `LC_MAX_BYTES`.

### `POST /corridas/validar` → `200`

Lo mismo, **sin guardar**. Es el endpoint contra el que se itera al conectar un pipeline nuevo.

```json
{ "valido": true, "avisos": ["`inicio` ausente o ilegible; se usó el ts del primer evento"],
  "leido": { "run_id": "...", "status": "ok" }, "etapas_detectadas": 3, "eventos": 12 }
```

## Consulta

### `GET /corridas`

| Parámetro | Qué hace |
|---|---|
| `proyecto`, `pipeline` | filtran; con los dos, la búsqueda va derecho al prefijo |
| `status` | `ok` · `sin_novedad` · `error` |
| `desde`, `hasta` | rango sobre el `run_id`, que es cronológico |
| `limite` | 50 por defecto |

### `GET /corridas/{run_id}`

El sobre normalizado, con todos sus eventos. Acepta `?proyecto=&pipeline=` como atajo: sin
ellos hay que buscar la corrida entre los resúmenes.

### `GET /corridas/{run_id}/vista`

**Lo que consume la pantalla principal.** La corrida ya reconstruida en etapas, con la salida
cruda separada de los eventos. Acepta `?nivel=warn` (de ese nivel para arriba) y `?etapa=...`.

```json
{ "corrida": {...},
  "etapas": [{ "nombre": "descargar", "ms": 4814, "estado": "ok",
               "eventos": [...], "salida": ["3 archivos"] }],
  "novedad": { "ev": "diff.resumen", "campos": {...} },
  "pipeline": {...} }
```

Las etapas se reconstruyen del par `stage.start`/`stage.end`; los eventos que caen fuera de
toda etapa van a una sintética `(corrida)`. El armado vive del lado del servidor: es
conocimiento del contrato, y el panel no tiene por qué repetirlo.

### `GET /corridas/{run_id}/bundle` → `text/plain`

El prompt de diagnóstico: la corrida, sus eventos, **la última corrida que sí funcionó** y el
código que el pipeline adjuntó. ~50 KB cuando la corrida trae contexto.

### `POST /corridas/{run_id}/diagnostico`

Llama al modelo con ese bundle. **Viene apagado**: sin `LC_DIAGNOSTICO=on` y una llave devuelve
`501` explicando cómo encenderlo.

```json
{ "modelo": "claude-opus-5", "texto": "...", "tokens_entrada": 12768,
  "tokens_salida": 900, "costo_usd": 0.0863, "segundos": 14.2 }
```

## Pipelines

### `GET /pipelines` · `GET /pipelines/{proyecto}/{pipeline}`

Cada uno con su última corrida y su estado **calculado a la hora de preguntar** —`estado_actual`
puede ser `en_silencio` aunque la última corrida diga `ok`, porque un `ok` de hace tres semanas
no es una buena noticia.

### `PUT /pipelines/{proyecto}/{pipeline}`

```json
{ "cadencia_h": 168, "evento_novedad": "diff.resumen", "descripcion": "el padrón" }
```

Lo único que decide el operador. No pisa nada de lo que reportaron los emisores.

### `DELETE /pipelines/{proyecto}/{pipeline}?confirmar={pipeline}`

Borra el pipeline entero: su estado, sus resúmenes y sus sobres.

```json
{ "eliminado": "leads-peak-shaving/etl-contratos",
  "objetos": { "sobres": 1, "resumenes": 1, "estado": 1 }, "total": 3 }
```

Existe por un caso concreto y repetible: un emisor mal configurado —un guion donde iba un guion
bajo— crea un pipeline fantasma que el panel muestra para siempre.

Dos guardas, y las dos importan:

- **Pide el token de EMISOR, no el de lector.** El panel solo tiene el del lector, así que la
  credencial que vive en tu navegador no puede destruir nada. Borrar es una operación de
  terminal, deliberadamente.
- **Hay que repetir el nombre en `confirmar`**, como en `terraform destroy`. Un `curl` mal
  copiado no debería poder vaciar la historia de un pipeline sano.

Borra también los sobres crudos. Es deliberado: un fantasma del que quedan los crudos vuelve a
aparecer en cuanto alguien reconstruya el índice. **No se deshace.**

## Salud y servicio

| Ruta | Qué devuelve |
|---|---|
| `GET /salud` | silencios, últimas corridas fallidas, y `sano` |
| `GET /contrato` | el documento de integración, sin token |
| `GET /check` | vivo (para un balanceador) |
| `GET /health` | sano: versión, entorno, tiempo arriba |
