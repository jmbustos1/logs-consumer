# Operación

## Correr

```bash
docker compose up --build             # todo
cd backend && go run ./cmd/api        # solo la API
cd backend && go run ./cmd/vigia      # el chequeo de silencio, suelto (sale 1 si algo anda mal)
./pruebas.sh                          # los criterios de aceptación
```

## Síntoma → causa probable → qué hacer

| Síntoma | Causa probable | Qué hacer |
|---|---|---|
| El pipeline dice `envio.error` y sigue | El consumer está caído o la URL está mal | **Nada urgente.** Es telemetría: la corrida terminó bien y su `.jsonl` está en disco. Se reenvía con `clientes/python/reenviar_jsonl.py` |
| `401` al mandar una corrida | Token de emisor mal o ausente | Comparar con `LC_TOKEN_EMISOR`. Ojo: el token del lector **no** sirve para escribir, a propósito |
| `413` | El sobre supera `LC_MAX_BYTES` | Casi siempre es `contexto` con archivos que no hacen falta. Adjuntar solo el código del pipeline, no el dataset |
| El emisor ve «connection reset» en vez de `413` | El cliente no manda `Expect: 100-continue`, así que sigue subiendo cuando el servidor ya respondió | Cosmético: el emisor lo trata como `warn` igual. `curl` sí ve el `413` |
| `400: falta el campo obligatorio` | El sobre no trae `proyecto`, `pipeline` o `run_id` | Probar contra `POST /corridas/validar`, que dice exactamente qué falta |
| `400: solo admite letras, dígitos…` | Alguno de esos tres campos trae `/`, espacios o `..` | Son los que forman rutas de archivo y claves de S3. Se rechazan a propósito |
| Un pipeline aparece `en_silencio` y sí corrió | El reloj del emisor está desfasado, o la cadencia declarada es más corta que la real | El aviso de la ingesta lo dice cuando el `inicio` viene del futuro. Ajustar `cadencia_h` con `PUT /pipelines/...` |
| Un pipeline nunca aparece `en_silencio` | No declaró cadencia | Es el comportamiento buscado: sin cadencia no se vigila. `PUT` con `cadencia_h` |
| La vista de corrida está vacía | El sobre llegó sin `eventos` | La ingesta lo avisa. Sin eventos hay corrida pero no hay qué mostrar |
| Las etapas no se agrupan | El emisor no manda `stage.start`/`stage.end`, o los manda sin `etapa`/`stage` | Todo cae en la etapa sintética `(corrida)`. Ver `INTEGRACION.md` § los eventos |
| El bundle sale sin código | La corrida no trajo `contexto` | Los pipelines lo adjuntan **solo cuando fallan**. En una corrida `ok` es lo esperado |
| `501` al pedir un diagnóstico | Viene apagado | `LC_DIAGNOSTICO=on` + `ANTHROPIC_API_KEY`. Mientras tanto, el bundle se copia gratis |
| El panel no muestra nada y la consola dice CORS | El origen del panel no está en `LC_CORS` | Agregarlo. En AWS no pasa: CloudFront sirve panel y API bajo el mismo dominio |
| Aparece un pipeline duplicado, con guion y con guion bajo | El emisor manda un `ETL_PIPELINE` distinto del que usabas antes (típico: Terraform nombra los recursos con guion y pasa ese nombre) | Corregir el emisor. Para la entrada huérfana, ver «borrar un pipeline» abajo — y antes de borrar, fijate si tiene corridas que no existan en otro lado |
| `arranque.sin_token` en el log | No hay tokens definidos | En local está bien. Antes de exponerlo, definir los dos |

## Dónde mirar

**El log del servicio** es una línea JSON por evento, a stdout — en AWS lo captura CloudWatch:

```bash
docker compose logs api | grep corrida.recibida
docker compose logs api | grep '"level":"ERROR"'
```

**El almacén** se puede leer a mano; esa es una de las razones de que no haya base de datos:

```bash
ls -R datos/                                        # el árbol completo
ls datos/resumen/<proyecto>/<pipeline>/             # el estado va en el NOMBRE del archivo
zcat datos/sobres/<proyecto>/<pipeline>/<run>.json.gz | python3 -m json.tool
aws s3 ls s3://$LC_BUCKET/resumen/ --recursive      # lo mismo en AWS
```

## Borrar un pipeline

```bash
curl -X DELETE "$LC_URL/pipelines/<proyecto>/<pipeline>?confirmar=<pipeline>" \
  -H "Authorization: Bearer $LC_TOKEN_EMISOR"
```

Pide el token de **emisor** —no el del panel— y que repitas el nombre. Se lleva los sobres
crudos, así que **no se deshace**.

Antes de borrar un fantasma, mirá qué corridas tiene: una tarea de Fargate no deja log local,
así que el sobre puede ser el único registro que quede de esa ejecución. Si vale la pena
conservarla, reingresala con la etiqueta correcta antes de borrar:

```bash
aws s3 cp s3://$LC_BUCKET/sobres/<proyecto>/<viejo>/<run>.json.gz - | gunzip \
  | python3 -c 'import json,sys; s=json.load(sys.stdin); s["pipeline"]="<nuevo>"; print(json.dumps(s))' \
  | curl -X POST "$LC_URL/corridas" -H "Authorization: Bearer $LC_TOKEN_EMISOR" \
      -H 'Content-Type: application/json' --data-binary @-
```

Se cambia **solo** la etiqueta de ruteo. Los eventos quedan intactos, diciendo lo que el emisor
realmente reportó: reescribirlos sería falsificar el registro para que cuadre con la corrección.

## Reconstruir

El sobre crudo es la fuente de verdad: todo lo demás sale de ahí. Si un resumen o el estado de
un pipeline se corrompe, se regenera reenviando los sobres:

```bash
for f in datos/sobres/*/*/*.json.gz; do
  zcat "$f" | curl -s -X POST "$LC_URL/corridas" -H 'Content-Type: application/json' --data-binary @- >/dev/null
done
```

## Desplegar

```bash
cd infra/terraform && cp terraform.tfvars.example terraform.tfvars   # completar
terraform init && terraform plan
terraform apply                     # crea recursos: es una decisión, no un paso más
```

Después, lo que imprime `terraform output siguiente_paso`: cargar los secretos en SSM, empujar
la imagen a ECR y sincronizar el panel a S3. Los secretos **no** pasan por Terraform: quedarían
en el tfstate, que es texto plano.

## Mantención

- **`INTEGRACION.md` está duplicado** en `backend/internal/handler/contrato.md`, porque la API
  lo sirve embebido en el binario y `go:embed` no puede salir del módulo. `pruebas.sh` verifica
  que no hayan divergido: si falla esa prueba, `cp INTEGRACION.md backend/internal/handler/contrato.md`.
- **Las versiones de los SDK están fijadas** por la toolchain de Go local. Ver
  [`DECISIONES.md`](DECISIONES.md) § versiones fijadas antes de subirlas.
