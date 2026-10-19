# Multi-stage: se compila con la toolchain completa y se despacha un binario estático sobre
# alpine. La imagen final ronda los 25 MB.
#
# Go 1.22 y no una versión más nueva: es la toolchain de la máquina de desarrollo, y el SDK de
# Anthropic está fijado en v1.9.0 por lo mismo. Cuando actualices Go, subí las dos cosas juntas
# — que la imagen compile con algo que localmente no compila es cómo se llega a "en mi máquina
# anda".
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO apagado: binario estático, sin sorpresas de libc entre alpine y distroless.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o api  ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vigia ./cmd/vigia

FROM alpine:3.20 AS servidor

# Certificados: hacen falta para hablar con S3 y con la API de Claude.
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 consumer

WORKDIR /app
COPY --from=builder /app/api   /app/api
COPY --from=builder /app/vigia /app/vigia

# El almacén en disco vive acá; en compose es un volumen y en AWS lo reemplaza S3.
RUN mkdir -p /datos && chown consumer:consumer /datos
ENV LC_DATOS=/datos LC_PUERTO=8080
VOLUME ["/datos"]

USER consumer
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/check || exit 1

CMD ["/app/api"]

# ── objetivo `lambda` ────────────────────────────────────────────────────────
# Lambda no ejecuta un servidor HTTP: entrega eventos por su Runtime API. Con aws-lambda-go el
# binario ES el handler, pero la imagen tiene que salir de la base `provided`, que trae el
# cliente del runtime. Es el mismo binario que arriba —el arranque detecta el entorno y
# envuelve el mismo http.Handler—, solo cambia el envoltorio.
#
#   docker build --target lambda -t logs-consumer:lambda .
FROM public.ecr.aws/lambda/provided:al2023 AS lambda
COPY --from=builder /app/api ${LAMBDA_TASK_ROOT}/bootstrap
CMD ["bootstrap"]
