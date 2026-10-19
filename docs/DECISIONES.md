# Por qué está hecho así

Las decisiones que no se deducen del código, con lo que costaría revertirlas.

## Sin base de datos

El almacén es un árbol de objetos, en disco localmente y en S3 en AWS, con el mismo layout de
claves en los dos lados.

Dos propiedades lo hacen suficiente:

1. **`run_id` ordena solo.** `AAAAMMDD-HHMMSS-hex` es lexicográficamente cronológico, así que
   «las últimas 50 corridas» es listar un prefijo y cortar el final.
2. **El estado va en el nombre del objeto** (`...<run_id>.error.json`), así que «los fallos de
   la semana» se responde listando claves, **sin abrir un solo objeto**.

**DynamoDB** se descartó por simplicidad, no por precio: a 150 corridas al mes costaría
centavos. Sin base de datos hay un servicio menos en compose, un módulo menos de Terraform, y
`ls -R datos/` sigue siendo una herramienta de depuración válida. **RDS** sí es caro: ~USD 15 al
mes, cuarenta veces lo que cuestan los tres pipelines que este servicio vigila.

**Cuándo revisarlo:** cuando listar empiece a doler. El puerto `domain.Almacen` está para meter
un índice detrás sin tocar los casos de uso — y recién ahí.

## Go en Lambda, no un contenedor encendido

El tráfico son ~150 POST al mes más las consultas de cuando alguien abre el panel. Un servicio
en Fargate detrás de un ALB costaría ~USD 25/mes por estar esperando.

La Function URL evita API Gateway, que aportaría claves de API y transformaciones que acá no se
usan y cobra aparte. La autenticación la hace la aplicación con sus dos tokens.

**El binario es el mismo en los dos entornos**: detecta que corre en Lambda y envuelve el mismo
`http.Handler` que usa `ListenAndServe` en local. No hay una segunda versión de las rutas que se
pueda desincronizar. Si algún día hace falta que esté siempre encendido, esa misma imagen se va
a Fargate sin tocar código.

## Panel estático, sin SSR

No hay SEO que ganar en un panel privado. `output: 'export'` produce HTML y JS que se sirven
desde S3+CloudFront por centavos, y la costura con el backend es una sola variable
(`NEXT_PUBLIC_API_URL`). En producción CloudFront sirve el panel y la API bajo el mismo dominio,
así que no hay CORS que configurar.

## Un solo dominio en CloudFront, y `/api/*` sin caché

Un panel que muestra corridas viejas es peor que uno lento. La política `CachingDisabled` en
`/api/*`, y `AllViewerExceptHostHeader` para que el `Authorization` llegue al backend — la
política por defecto lo descarta.

## Arquitectura clean, pero solo donde paga

Hay puertos en el dominio (`Almacen`, `Repositorio`, `Notificador`, `Diagnosticador`) porque
cada uno tiene **dos implementaciones reales o previstas**: disco y S3; ningún canal de alertas
y el que venga; el modelo apagado y encendido. Donde no hay dos implementaciones no hay
interfaz: los handlers llaman a los casos de uso y listo.

El dominio y los casos de uso no importan nada de infraestructura, así que `go test ./...` corre
sin levantar nada.

## El diagnóstico viene apagado

El código está escrito y probado; el interruptor es `LC_DIAGNOSTICO=on` más
`ANTHROPIC_API_KEY` —el mismo nombre de variable que usa lids-searcher, porque es la misma
llave—. Nunca es automático: se dispara con un clic, para que un pipeline que falla en bucle no
gaste solo. Cada llamada registra su costo estimado, con la tabla de precios por modelo.

El camino por defecto es `GET .../bundle`: el mismo contexto, gratis, para pegar a mano.

## `sin_novedad` no es un fallo

Es el estado que se malinterpreta y por eso está escrito en tres lugares. Un pipeline que dice
`sin_novedad` corrió bien y no había nada que hacer: la fuente no republicó. Sale con código 0.
Pintarlo de rojo llena el panel de alarmas, y un panel donde todo está rojo es un panel que se
deja de mirar.

## La cadencia la declara quien vigila

Un pipeline no sabe cada cuánto *debería* correr; sabe cuándo lo corrieron. La cadencia esperada
se declara con `PUT /pipelines/...` y es lo único que permite detectar la falla que no manda
ningún sobre: la que mata el proceso antes de que pueda hablar.

Un pipeline **sin** cadencia declarada no se vigila, a propósito: conectar un emisor nuevo no
puede llenar el panel de ámbar.

## Ingesta tolerante, y qué no se tolera

Se exige el mínimo (`proyecto`, `pipeline`, `run_id`) y se preserva todo lo demás. `stage` y
`etapa` son lo mismo; `level` y `lvl` también; los campos desconocidos se guardan. Si conectar
un emisor nuevo exigiera negociar un esquema, nadie conectaría el segundo.

Lo que **no** se tolera son esos tres campos con caracteres raros: forman rutas de archivo y
claves de S3, y un `..` ahí sería escritura arbitraria.

Y un `status` desconocido **no se mapea**: se conserva tal cual con un aviso. Mandarlo a `ok`
escondería un fallo; mandarlo a `error` inventaría uno.

## Versiones fijadas

La máquina de desarrollo tiene **Go 1.22** y la descarga automática de toolchains está
bloqueada. Los SDK más nuevos exigen Go 1.24, así que están fijados en las últimas versiones que
compilan con 1.22:

| Módulo | Fijado en | Lo último exige |
|---|---|---|
| `anthropic-sdk-go` | v1.9.0 | Go 1.24 |
| `aws-sdk-go-v2/service/s3` | v1.71.0 | Go 1.24 |
| `aws-sdk-go-v2/config` | v1.28.0 | Go 1.24 |
| `aws-sdk-go-v2/service/ssm` | v1.56.0 | Go 1.24 |
| `aws-lambda-go` + `aws-lambda-go-api-proxy` | v1.47.0 / v0.16.2 | — |

El `Dockerfile` usa `golang:1.22-alpine` por lo mismo: que la imagen compile con algo que
localmente no compila es exactamente cómo se llega a «en mi máquina anda».

**Cuando actualices Go**, subí las dos cosas juntas: la imagen base y los pines.

Nada de esto limita las capacidades usadas. El modelo por defecto es `claude-opus-5` —un string,
no una constante del SDK— y en Opus 5 el pensamiento adaptativo está encendido de fábrica, así
que no hace falta configurarlo.

## Lo que se probó de verdad

- Las **26 corridas reales** de los tres ETL de `leads-searcher-peak-shavin`, reconstruidas
  desde sus `.jsonl`.
- **Dos corridas nuevas de `etl_contratos`** contra este servicio, una normal y una forzada a
  fallar: la segunda llegó con sus 52 KB de `contexto` y produjo un bundle de ~12.700 tokens.
- Los **tres clientes**: Python (los ETL reales), Go y bash, cada uno emitiendo el mismo
  contrato.
- La API **dentro del contenedor**, y la imagen de Lambda construida.
- `pruebas.sh`: 28 criterios en verde.
