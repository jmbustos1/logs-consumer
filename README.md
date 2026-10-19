# logs-consumer

Recolector de corridas de pipelines. Recibe un POST por corrida, muestra qué hizo cada etapa, y
avisa cuándo un pipeline dejó de hablar.

```
┌─ pipelines ─────────┐   POST /api/v1/corridas    ┌─ backend Go ────────────┐
│ etl_contratos       │ ─────────────────────────► │ handler → usecase       │
│ etl_mensual         │   Bearer <token emisor>    │        ↘ dominio        │
│ etl_resolucion      │                            │ adapters: disco · s3    │
│ lids-searcher       │                            └────────────┬────────────┘
│ (el próximo)        │                                         │ GET /api/v1/*
└─────────────────────┘                            ┌────────────┴────────────┐
                                                   │ panel Next estático     │
                                                   │ S3 + CloudFront         │
                                                   └─────────────────────────┘
```

## Qué resuelve

**Ver la ejecución por dentro.** Etapa por etapa, con su duración, sus contadores y la salida
cruda de los subprocesos. Antes eso significaba entrar a la máquina donde corrió y leer un
`.jsonl` a mano.

**Notar el silencio.** Un pipeline que falla manda su sobre y se nota. Uno que muere duro —OOM,
tarea matada, cron desactivado— no manda nada, y la ausencia es la única señal que queda. Por
eso cada pipeline declara cada cuánto *debería* correr.

**Armar el bundle de diagnóstico.** Cuando una corrida falla, el pipeline adjunta su código y
sus documentos. El consumer los junta con los eventos y **con la última corrida que sí
funcionó** en un texto pegable en una sesión de Claude Code.

## Levantarlo

```bash
docker compose up --build          # API en :8787, panel en :3000
```

O sin Docker:

```bash
cd backend  && go run ./cmd/api                      # :8080
cd frontend && npm install && npm run dev            # :3000
```

Y para que empiece a llegar, en cualquier pipeline:

```bash
export ETL_LOGS_URL=http://localhost:8787/api/v1/corridas
```

Si ya tenés corridas viejas en disco, se pueden importar:

```bash
python3 clientes/python/reenviar_jsonl.py \
  --url http://localhost:8787/api/v1/corridas ~/proyecto/logs/*.jsonl
```

## Conectar un pipeline

Todo lo necesario está en **[`INTEGRACION.md`](INTEGRACION.md)** — y la propia API sirve una
copia en `GET /api/v1/contrato`, para que un agente pueda leerla sin tener este repositorio.
Hay clientes copiables en `clientes/` para Python, Go y bash.

## Estructura

```
backend/          Go: la API. cmd · config · internal/{domain,usecase,adapter,handler,router}
frontend/         Next estático: el panel, con la paleta de AETERN
clientes/         qué copiar al pipeline que se quiere conectar
infra/terraform/  ECR · Lambda · S3 ×2 · CloudFront · SSM · IAM · presupuesto
docs/             API, operación y por qué está hecho así
pruebas.sh        los criterios de aceptación, ejecutables
INTEGRACION.md    el contrato, para quien conecta un pipeline
```

## Desplegar en AWS

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars    # solo pide un correo para las alertas
terraform init && terraform plan
terraform apply
```

**El repositorio no contiene nada que identifique una cuenta.** Los nombres de bucket —que
tienen que ser únicos en todo AWS— se derivan del id de cuenta de las credenciales con las que
corras Terraform, no de un valor escrito en un archivo. Los secretos se crean vacíos en SSM y se
cargan con la CLI, porque si pasaran por Terraform quedarían en el `tfstate`.

El resto de los pasos —cargar los tokens, empujar la imagen, subir el panel— los imprime
`terraform output siguiente_paso`, ya con tus valores. Detalle en
[`docs/OPERACION.md`](docs/OPERACION.md).

## Configuración

| Variable | Default | Qué hace |
|---|---|---|
| `LC_PUERTO` | `8080` | |
| `LC_ALMACEN` | `disco` | `disco` o `s3` |
| `LC_DATOS` | `./datos` | raíz del almacén en disco |
| `LC_BUCKET` | | bucket, si `LC_ALMACEN=s3` |
| `LC_TOKEN_EMISOR` | *(vacío)* | Bearer para escribir. Vacío = ruta abierta |
| `LC_TOKEN_LECTOR` | *(vacío)* | Bearer para leer. Vacío = ruta abierta |
| `LC_SSM_PREFIJO` | *(vacío)* | si está, los tokens salen de SSM |
| `LC_MAX_BYTES` | `8388608` | tope del sobre |
| `LC_CORS` | `http://localhost:3000` | orígenes permitidos, separados por coma |
| `LC_DIAGNOSTICO` | `off` | `on` enciende la llamada al modelo |
| `ANTHROPIC_API_KEY` | *(vacío)* | mismo nombre que en lids-searcher: es la misma llave |
| `LC_MODELO` | `claude-opus-5` | |

## Estado

| | |
|---|---|
| Backend | ✅ API completa, 34 criterios de aceptación en verde, imagen de 29 MB |
| Panel | ✅ portada, historial, vista de corrida y salud; export estático |
| Clientes | ✅ Python, Go y bash, los tres probados contra la API |
| Infra | ✅ **desplegada**: Lambda + Function URL, dos buckets, CloudFront |
| En uso | ✅ 5 pipelines de 2 proyectos reportando desde Fargate |
| Costo real | ✅ por debajo del redondeo de Cost Explorer |
| Alertas | ⛔ no hay canal. `Notificador` es el enchufe |
| Diagnóstico con el modelo | 🔌 escrito y probado, **apagado** por defecto |

## Lo que no hace

- **No hay alertas todavía**: enterarse requiere abrir el panel.
- **No llama al modelo salvo que lo enciendas.** El camino por defecto es copiar el bundle.
- **No grafica series de tiempo.** Con unas decenas de corridas no hay nada que graficar.
- **No hay usuarios ni roles.** Dos tokens: uno escribe, otro lee.
