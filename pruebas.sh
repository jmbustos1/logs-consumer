#!/usr/bin/env bash
# Criterios de aceptación, ejecutables. Levanta el servicio contra un almacén temporal, lo
# somete a los casos que importan y lo apaga.
#
#   ./pruebas.sh                    contra un binario recién compilado
#   API=http://otro:8080 ./pruebas.sh   contra un servicio ya levantado
set -uo pipefail
cd "$(dirname "$0")"

ok=0; fallidas=0
prueba() { # prueba "descripción" "esperado" "obtenido"
  if [ "$2" = "$3" ]; then printf '  \033[32m✓\033[0m %-52s %s\n' "$1" "$3"; ok=$((ok+1))
  else printf '  \033[31m✗\033[0m %-52s esperado=%s obtenido=%s\n' "$1" "$2" "$3"; fallidas=$((fallidas+1)); fi
}
contiene() {
  if printf '%s' "$3" | grep -q "$2"; then printf '  \033[32m✓\033[0m %-52s\n' "$1"; ok=$((ok+1))
  else printf '  \033[31m✗\033[0m %-52s no contiene %q\n' "$1" "$2"; fallidas=$((fallidas+1)); fi
}
codigo() { curl -s -o /tmp/lc-prueba-cuerpo -w '%{http_code}' "$@"; }

# ── arranque ─────────────────────────────────────────────────────────────────
PROPIO=0
if [ -z "${API:-}" ]; then
  DATOS="$(mktemp -d)"; PUERTO=8799; API="http://localhost:$PUERTO"
  echo "  compilando…"
  (cd backend && go build -o /tmp/lc-pruebas ./cmd/api) || exit 1
  LC_DATOS="$DATOS" LC_PUERTO="$PUERTO" /tmp/lc-pruebas > /tmp/lc-pruebas.log 2>&1 &
  PID=$!; PROPIO=1
  trap 'kill $PID 2>/dev/null; rm -rf "$DATOS"' EXIT
  for _ in $(seq 20); do curl -sf "$API/check" >/dev/null 2>&1 && break; sleep 0.3; done
fi
V1="$API/api/v1"
echo

echo "── pruebas unitarias ────────────────────────────────────────────────"
if (cd backend && go test ./... >/tmp/lc-gotest.log 2>&1); then
  prueba "go test ./..." "ok" "ok"
else
  prueba "go test ./..." "ok" "falló"; tail -20 /tmp/lc-gotest.log
fi
echo

echo "── el contrato ──────────────────────────────────────────────────────"
AHORA="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
SOBRE='{"proyecto":"pruebas","pipeline":"demo","run_id":"20260831-120000-aaa","status":"ok",
 "inicio":"'"$AHORA"'","segundos":2.5,"contadores":{"filas":10},
 "eventos":[{"ts":"2026-08-31T12:00:00Z","lvl":"info","ev":"stage.start","etapa":"bajar"},
            {"ts":"2026-08-31T12:00:01Z","lvl":"debug","ev":"etapa.salida","etapa":"bajar","linea":"3 archivos"},
            {"ts":"2026-08-31T12:00:02Z","lvl":"info","ev":"stage.end","etapa":"bajar","ms":2000}]}'

prueba "corrida válida se acepta" "202" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d "$SOBRE")"

prueba "reenviarla no la duplica" "1" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d "$SOBRE" >/dev/null; \
     curl -s "$V1/corridas?proyecto=pruebas" | grep -o '"total":[0-9]*' | cut -d: -f2)"

prueba "sin run_id: 400" "400" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d '{"proyecto":"p","pipeline":"q"}')"
contiene "…y dice qué campo falta" "run_id" "$(cat /tmp/lc-prueba-cuerpo)"

prueba "JSON roto: 400" "400" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d '{roto')"

prueba "intento de traversal en proyecto: 400" "400" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' \
     -d '{"proyecto":"../../etc","pipeline":"q","run_id":"r"}')"

prueba "cuerpo vacío: 400" "400" "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d '')"

# El sobre gigante se rechaza por Content-Length, antes de leer un byte.
head -c 9000000 /dev/zero | tr '\0' 'x' > /tmp/lc-relleno
printf '{"proyecto":"p","pipeline":"q","run_id":"r","relleno":"%s"}' "$(cat /tmp/lc-relleno)" > /tmp/lc-gigante.json
prueba "sobre por encima del límite: 413" "413" \
  "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' --data-binary @/tmp/lc-gigante.json)"

prueba "validar no guarda nada" "1" \
  "$(codigo -X POST "$V1/corridas/validar" -H 'Content-Type: application/json' \
      -d '{"proyecto":"pruebas","pipeline":"otro","run_id":"20260831-130000-bbb"}' >/dev/null; \
     curl -s "$V1/corridas?proyecto=pruebas" | grep -o '"total":[0-9]*' | cut -d: -f2)"
contiene "…y devuelve avisos útiles" "avisos" "$(cat /tmp/lc-prueba-cuerpo)"
echo

echo "── la vista de corrida ──────────────────────────────────────────────"
VISTA="$(curl -s "$V1/corridas/20260831-120000-aaa/vista")"
contiene "reconstruye las etapas"           '"nombre":"bajar"'  "$VISTA"
contiene "con su duración"                  '"ms":2000'         "$VISTA"
contiene "y la salida cruda del subproceso" '3 archivos'        "$VISTA"
contiene "el bundle se arma"                'Qué se te pide'    "$(curl -s "$V1/corridas/20260831-120000-aaa/bundle")"
prueba "corrida inexistente: 404" "404" "$(codigo "$V1/corridas/no-existe/vista")"
echo

echo "── el vigía ─────────────────────────────────────────────────────────"
# Dos pipelines distintos, uno fresco y uno viejo. Antes esto usaba una cadencia de 0,36 s
# sobre el mismo pipeline y competía contra el reloj del envío: fallaba una vez cada tantas.
# Una prueba que a veces miente es peor que no tenerla.
curl -s -X PUT "$V1/pipelines/pruebas/demo" -H 'Content-Type: application/json' \
  -d '{"cadencia_h":24,"evento_novedad":"diff.resumen"}' >/dev/null

VIEJO='{"proyecto":"pruebas","pipeline":"abandonado","run_id":"20200101-000000-aaa","status":"ok",
 "inicio":"2020-01-01T00:00:00Z","segundos":1,
 "eventos":[{"ts":"2020-01-01T00:00:00Z","lvl":"info","ev":"run.start"}]}'
curl -s -X POST "$V1/corridas" -H 'Content-Type: application/json' -d "$VIEJO" >/dev/null
curl -s -X PUT "$V1/pipelines/pruebas/abandonado" -H 'Content-Type: application/json' \
  -d '{"cadencia_h":24}' >/dev/null

prueba "el que corrió recién NO está en silencio" "False" \
  "$(curl -s "$V1/salud" | python3 -c 'import json,sys; print(any(p["pipeline"]=="demo" for p in json.load(sys.stdin)["silencios"]))')"
prueba "el abandonado SÍ está en silencio" "True" \
  "$(curl -s "$V1/salud" | python3 -c 'import json,sys; print(any(p["pipeline"]=="abandonado" for p in json.load(sys.stdin)["silencios"]))')"
echo

echo "── borrar un pipeline ───────────────────────────────────────────────"
prueba "sin confirmar: 400" "400" "$(codigo -X DELETE "$V1/pipelines/pruebas/abandonado")"
prueba "confirmando mal: 400" "400" "$(codigo -X DELETE "$V1/pipelines/pruebas/abandonado?confirmar=otracosa")"
prueba "pipeline inexistente: 404" "404" "$(codigo -X DELETE "$V1/pipelines/pruebas/fantasma?confirmar=fantasma")"
prueba "confirmando bien: 200" "200" "$(codigo -X DELETE "$V1/pipelines/pruebas/abandonado?confirmar=abandonado")"
prueba "…y desaparece del panel" "False" \
  "$(curl -s "$V1/pipelines" | python3 -c 'import json,sys; print(any(p["pipeline"]=="abandonado" for p in json.load(sys.stdin)["pipelines"]))')"
prueba "…sin tocar a los vecinos" "True" \
  "$(curl -s "$V1/pipelines" | python3 -c 'import json,sys; print(any(p["pipeline"]=="demo" for p in json.load(sys.stdin)["pipelines"]))')"
echo

echo "── el diagnóstico viene apagado ─────────────────────────────────────"
prueba "diagnóstico desactivado: 501" "501" "$(codigo -X POST "$V1/corridas/20260831-120000-aaa/diagnostico")"
contiene "…y explica cómo encenderlo" "LC_DIAGNOSTICO=on" "$(cat /tmp/lc-prueba-cuerpo)"
echo

echo "── el contrato lo sirve la API ──────────────────────────────────────"
prueba "GET /contrato sin token" "200" "$(codigo "$V1/contrato")"
contiene "…y es el documento de integración" "El sobre" "$(cat /tmp/lc-prueba-cuerpo)"
if [ -f INTEGRACION.md ]; then
  if diff -q INTEGRACION.md backend/internal/handler/contrato.md >/dev/null; then
    prueba "INTEGRACION.md y la copia embebida coinciden" "iguales" "iguales"
  else
    prueba "INTEGRACION.md y la copia embebida coinciden" "iguales" "DIVERGIERON"
  fi
fi
echo

echo "── tokens ───────────────────────────────────────────────────────────"
if [ "$PROPIO" = "1" ]; then
  kill $PID 2>/dev/null; sleep 0.5
  LC_DATOS="$DATOS" LC_PUERTO="$PUERTO" LC_TOKEN_EMISOR=secreto-emisor LC_TOKEN_LECTOR=secreto-lector \
    /tmp/lc-pruebas >> /tmp/lc-pruebas.log 2>&1 &
  PID=$!
  for _ in $(seq 20); do curl -sf "$API/check" >/dev/null 2>&1 && break; sleep 0.3; done

  prueba "ingesta sin token: 401" "401" \
    "$(codigo -X POST "$V1/corridas" -H 'Content-Type: application/json' -d "$SOBRE")"
  prueba "ingesta con el token del lector: 401" "401" \
    "$(codigo -X POST "$V1/corridas" -H 'Authorization: Bearer secreto-lector' \
       -H 'Content-Type: application/json' -d "$SOBRE")"
  prueba "ingesta con su token: 202" "202" \
    "$(codigo -X POST "$V1/corridas" -H 'Authorization: Bearer secreto-emisor' \
       -H 'Content-Type: application/json' -d "$SOBRE")"
  prueba "lectura sin token: 401" "401" "$(codigo "$V1/pipelines")"
  prueba "el contrato sigue abierto" "200" "$(codigo "$V1/contrato")"
fi
echo

printf '  %d pasaron · %d fallaron\n' "$ok" "$fallidas"
[ "$fallidas" -eq 0 ]
