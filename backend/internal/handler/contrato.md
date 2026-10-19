# Conectar un pipeline a logs-consumer

> Este documento se basta solo. Si estás conectando un pipeline nuevo —humano o agente— no
> necesitás leer nada más, ni tener este repositorio a mano: la propia API sirve una copia en
> `GET /api/v1/contrato`.

El consumer recibe **un POST por corrida**, al final, con un sobre JSON. Nada más. No hay
agente que instalar, no hay demonio, no hay streaming.

---

## En 5 pasos

```
1. copiar un cliente de clientes/  ->  al pipeline
2. definir LC_URL y LC_TOKEN       ->  en su entorno
3. emitir eventos por etapa        ->  el pipeline ya loguea; hay que estructurarlo
4. probar contra /corridas/validar ->  hasta que no queden avisos
5. declarar la cadencia            ->  PUT /pipelines/{proyecto}/{pipeline}
```

El paso 4 es el que ahorra el tiempo: `POST /api/v1/corridas/validar` normaliza el sobre,
**no guarda nada** y devuelve la lista de avisos. Se itera contra él hasta que sale limpio.

---

## El sobre

```json
{
  "proyecto":   "leads-peak-shaving",
  "pipeline":   "etl_contratos",
  "run_id":     "20260831-061149-76d",
  "status":     "ok",
  "inicio":     "2026-08-31T06:11:49+00:00",
  "segundos":   7.5,
  "entorno":    "local",
  "version":    "1.0.0",
  "host":       "tecob-ThinkPad-T14s-Gen-5",
  "comando":    "etl_contratos",
  "contadores": { "validar.filas": 11053, "diff.nuevos": 0 },
  "eventos":    [ { "ts": "...", "lvl": "info", "ev": "stage.start", "etapa": "descargar" } ],
  "contexto":   [ { "archivo": "etl/pipeline.py", "bytes": 4210, "contenido": "..." } ]
}
```

### Campos

| Campo | ¿Obligatorio? | Qué es |
|---|---|---|
| `proyecto` | **sí** | agrupa pipelines del mismo producto. Letras, dígitos, `-`, `_`, `.` |
| `pipeline` | **sí** | el pipeline dentro de ese proyecto. Mismas reglas |
| `run_id` | **sí** | identifica la corrida. **Usá `AAAAMMDD-HHMMSS-hex`**: así ordena cronológicamente solo, y el consumer no necesita índice para listar por fecha |
| `status` | no (`ok`) | `ok` · `sin_novedad` · `error`. Ver abajo |
| `inicio` | no | ISO-8601. Si falta, se toma del primer evento |
| `segundos` | no | duración total |
| `entorno` | no | `local`, `fargate`, `lambda`… lo que uses |
| `version` | no | SHA de git o marca de versión: sirve para saber qué código corrió |
| `host`, `comando` | no | de dónde salió |
| `contadores` | no | **libres**. Cada pipeline manda los suyos; el panel los muestra tal cual |
| `eventos` | no, pero sin ellos no hay vista | el `.jsonl` de la corrida, línea por línea |
| `contexto` | no | archivos para diagnosticar. Ver «contexto» abajo |

### Los tres estados, y el que se malinterpreta

| Estado | Código de salida | Significa |
|---|---|---|
| `ok` | 0 | corrió e hizo algo |
| `sin_novedad` | 0 | **corrió bien y no había nada que hacer** — la fuente no republicó |
| `error` | 1 | falló |

`sin_novedad` **no es un fallo**. Un ETL mensual que dice `sin_novedad` durante tres semanas
está sano. Si lo mandás como `error`, el panel se llena de rojo, y un panel donde todo está
rojo es un panel que se deja de mirar.

Se aceptan sinónimos (`success`, `failed`, `no_change`) y se normalizan con un aviso. Un
`status` que no se reconoce **se conserva tal cual**: mapearlo a `ok` escondería un fallo y
mapearlo a `error` inventaría uno.

### Los eventos

Una línea por evento, como la escribiría el pipeline en su `.jsonl`:

```json
{"ts": "2026-08-31T06:11:51.284+00:00", "lvl": "info", "ev": "stage.end", "etapa": "descargar", "ms": 4814.2}
```

| Campo | Qué es |
|---|---|
| `ts` | ISO-8601 |
| `lvl` | `debug` · `info` · `warn` · `error` (también se acepta `level`) |
| `ev` | nombre del evento |
| `etapa` | a qué etapa pertenece (también se acepta `stage`, en inglés) |
| *cualquier otro* | se preserva y se muestra |

**Cuatro nombres de evento tienen significado para el consumer.** El resto es libre:

| Evento | Para qué lo usa el panel |
|---|---|
| `stage.start` | abre una etapa (con `etapa` o `stage`) |
| `stage.end` | la cierra; el campo `ms` es su duración |
| `stage.error` | la marca como fallida; el campo `error` es el motivo |
| `etapa.salida` | el campo `linea` es **una línea de salida cruda del subproceso** y se muestra en un bloque de consola |

Si tu pipeline envuelve subprocesos —un `.sh`, un `unzip`, un `python -m otracosa`— emitir una
`etapa.salida` por línea es lo que convierte el panel en un log de verdad en vez de un resumen.

### El contexto

Cuando una corrida falla, adjuntá lo que haría falta para arreglarla **sin conocer el
proyecto**: el código del pipeline y los documentos que explican lo que el código no dice.

```json
"contexto": [{"archivo": "etl/pipeline.py", "bytes": 4210, "contenido": "import ..."}]
```

El consumer lo convierte en el bundle (`GET /corridas/{run_id}/bundle`): un texto pegable en
una sesión de Claude Code, con la corrida, sus eventos, **la última corrida que sí funcionó**
y ese código.

Mandalo **solo cuando falla**. Un sobre `ok` pesa ~6 KB; uno con contexto, ~50 KB. Mandar el
código en cada corrida infla el 99 % de los mensajes por el 1 % que hace falta.

> **Nunca metas secretos en `contexto`.** Es el archivo tal cual, y va a quedar guardado y a
> viajar a un modelo si alguien pide un diagnóstico. Filtrá `.env`, llaves y credenciales.

---

## Las tres reglas del emisor

No son estilo. Cada una responde a una forma concreta de romper algo.

**1. El envío nunca puede tumbar el pipeline.** `try/except` amplio a propósito: si el consumer
está caído, con el certificado vencido o simplemente todavía no existe, la corrida **termina
bien** y el fallo del envío queda como un `warn` en el log local. Esto es telemetría, no el
trabajo.

**2. Un solo POST al final.** No hay streaming por evento. Ocho etapas una vez al mes no
justifican multiplicar los modos de falla, y un POST por evento convierte una caída de red en
una corrida a medias.

**3. El log local es la fuente de verdad.** El envío es una copia. Si el consumer pierde datos,
el `.jsonl` sigue en disco — y `clientes/python/reenviar_jsonl.py` lo vuelve a mandar.

---

## Autenticación

Dos tokens distintos, a propósito:

| Token | Para qué | Dónde va |
|---|---|---|
| **emisor** | escribir corridas | el pipeline, en `Authorization: Bearer` |
| **lector** | leer el panel | el navegador |

Que se filtre el token de un pipeline no debe abrir la lectura de todos los demás. Si el
consumer arranca sin tokens definidos, las rutas quedan abiertas — cómodo en local, y el
arranque lo avisa con un `warn`.

---

## Los clientes

| Archivo | Para |
|---|---|
| `clientes/python/envio.py` + `log.py` | pipelines en Python. Son los que ya corren en producción |
| `clientes/go/envio.go` | pipelines en Go |
| `clientes/bash/envio.sh` | pipelines que son un shell script |
| `clientes/python/reenviar_jsonl.py` | corridas viejas que solo existen como `.jsonl` en disco |

---

## Endpoints que te importan al integrar

| Método | Ruta | Para qué |
|---|---|---|
| POST | `/api/v1/corridas/validar` | probar el sobre sin guardarlo |
| POST | `/api/v1/corridas` | mandar la corrida (`202`) |
| PUT | `/api/v1/pipelines/{proyecto}/{pipeline}` | declarar cadencia y evento de novedad |
| GET | `/api/v1/contrato` | este documento, servido por la API |

### Declarar la cadencia

```bash
curl -X PUT "$LC_URL/pipelines/leads-peak-shaving/etl_contratos" \
  -H "Authorization: Bearer $LC_TOKEN_LECTOR" -H 'Content-Type: application/json' \
  -d '{"cadencia_h": 168, "evento_novedad": "diff.resumen"}'
```

- **`cadencia_h`** — cada cuánto *debería* correr. Si pasa `cadencia × 1,5` sin noticias, el
  pipeline queda `en_silencio`. Un pipeline sin cadencia declarada **no se vigila**: conectar
  uno nuevo no llena el panel de alarmas.
- **`evento_novedad`** — cuál de tus eventos cuenta algo del mundo y no del pipeline. De los
  nueve eventos que emite una corrida, ocho dicen si el ETL funcionó y uno dice si pasó algo
  (`diff.resumen`, `seleccion`, `precision`…). Ese va a la portada; el resto, al detalle.

**Por qué la cadencia la declara quien vigila y no el emisor:** un pipeline no sabe cada cuánto
"debería" correr; sabe cuándo lo corrieron. Y es lo único que permite detectar la única falla
que no manda ningún sobre: la que mata el proceso antes de que pueda hablar.

---

## Qué NO hacer

- **No hagas streaming por evento.** Un POST al final.
- **No rompas la corrida si el POST falla.** Es telemetría.
- **No mandes secretos en `contexto`.**
- **No inventes valores de `status`** fuera de los tres.
- **No uses un `run_id` que no ordene cronológicamente.** Todo el listado del consumer se apoya
  en que `run_id` ordena solo.
- **No reenvíes la misma corrida con distinto `status`** esperando dos entradas: la ingesta es
  idempotente por `run_id` y la segunda pisa a la primera.

---

## Prompt de integración

Pegá esto en la sesión del pipeline que querés conectar. Se basta solo:

```text
Conectá este pipeline a logs-consumer, un recolector de corridas.

1. Leé el contrato: curl -s $LC_URL/contrato
2. Copiá el cliente que corresponda al lenguaje del pipeline desde clientes/ del repo
   logs-consumer (python, go o bash).
3. Envolvé la ejecución para que emita, por etapa, eventos stage.start / stage.end /
   stage.error, y —si el pipeline llama subprocesos— una etapa.salida por línea de salida.
4. Al terminar, armá el sobre y mandalo con UN POST a $LC_URL/corridas.
   Reglas que no se negocian: el envío nunca puede romper la corrida (try/except amplio, el
   fallo se loguea como warn); un solo POST al final; el log local sigue siendo la fuente de
   verdad.
5. Si la corrida falla, adjuntá en `contexto` el código del pipeline y sus documentos de
   operación. Nunca secretos.
6. Probá con POST $LC_URL/corridas/validar hasta que no devuelva avisos.
7. Declará la cadencia con PUT $LC_URL/pipelines/{proyecto}/{pipeline}.

Usá `sin_novedad` cuando el pipeline corre bien y no había nada que hacer: NO es un fallo y
sale con código 0.
```
