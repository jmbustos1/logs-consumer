#!/usr/bin/env bash
# Cliente mínimo para pipelines que son un shell script.
#
# Uso: source este archivo, llamá lc_etapa/lc_evento durante la corrida y lc_enviar al final.
#
#   source envio.sh
#   lc_init "mi-proyecto" "mi_pipeline"
#   lc_etapa_inicio descargar
#     curl -sO https://... 2>&1 | lc_salida descargar
#   lc_etapa_fin descargar
#   lc_contador archivos 4
#   lc_enviar ok
#
# Las tres reglas: el envío nunca rompe la corrida, un solo POST al final, y el .jsonl local
# es la fuente de verdad. Por eso lc_enviar termina siempre en 0.
set -uo pipefail

LC_URL="${LC_URL:-}"
LC_TOKEN="${LC_TOKEN:-}"
LC_TIMEOUT="${LC_TIMEOUT:-5}"

_lc_run_id=""; _lc_proyecto=""; _lc_pipeline=""; _lc_log=""; _lc_t0=0
declare -A _lc_contadores
declare -A _lc_etapa_t0

lc_init() {
  _lc_proyecto="$1"; _lc_pipeline="$2"
  _lc_run_id="$(date -u +%Y%m%d-%H%M%S)-$(printf '%03x' $((RANDOM % 4096)))"
  _lc_t0=$(date +%s)
  _lc_log="${LC_LOGS:-./logs}/${_lc_run_id}.jsonl"
  mkdir -p "$(dirname "$_lc_log")"
  lc_evento run.start info "\"comando\":\"${0##*/}\",\"host\":\"$(hostname)\""
}

# lc_evento <nombre> <nivel> [pares json extra]
lc_evento() {
  local ev="$1" lvl="${2:-info}" extra="${3:-}"
  local ts; ts="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
  local linea="{\"ts\":\"$ts\",\"lvl\":\"$lvl\",\"ev\":\"$ev\""
  [ -n "$extra" ] && linea="$linea,$extra"
  echo "$linea}" >> "$_lc_log"
}

lc_etapa_inicio() { _lc_etapa_t0["$1"]=$(date +%s%3N); lc_evento stage.start info "\"etapa\":\"$1\""; }
lc_etapa_fin()    { local ms=$(( $(date +%s%3N) - ${_lc_etapa_t0[$1]:-0} )); lc_evento stage.end info "\"etapa\":\"$1\",\"ms\":$ms"; }
lc_etapa_error()  { local ms=$(( $(date +%s%3N) - ${_lc_etapa_t0[$1]:-0} )); lc_evento stage.error error "\"etapa\":\"$1\",\"ms\":$ms,\"error\":$(lc_json "$2")"; }

# lc_salida <etapa> — se usa como tubería: comando 2>&1 | lc_salida descargar
# Cada línea del subproceso se convierte en un evento etapa.salida, que el panel muestra
# como bloque de consola. Es lo que hace que la vista sea un log y no un resumen.
lc_salida() {
  local etapa="$1"
  while IFS= read -r l; do
    printf '%s\n' "$l"
    lc_evento etapa.salida debug "\"etapa\":\"$etapa\",\"linea\":$(lc_json "$l")"
  done
}

lc_contador() { _lc_contadores["$1"]="$2"; }

lc_json() { python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$1" 2>/dev/null || echo '""'; }

# lc_enviar <ok|sin_novedad|error> [archivos de contexto...]
lc_enviar() {
  local status="${1:-ok}"; shift || true
  local segundos=$(( $(date +%s) - _lc_t0 ))
  lc_evento run.end info "\"status\":\"$status\",\"segundos\":$segundos"

  local sobre
  sobre="$(LC_STATUS="$status" LC_SEG="$segundos" LC_PROY="$_lc_proyecto" LC_PIPE="$_lc_pipeline" \
    LC_RUN="$_lc_run_id" LC_LOGF="$_lc_log" LC_CTX="$*" \
    LC_CONT="$(for k in "${!_lc_contadores[@]}"; do echo "$k=${_lc_contadores[$k]}"; done)" \
    python3 - <<'PY'
import json, os, pathlib
eventos = []
for linea in pathlib.Path(os.environ["LC_LOGF"]).read_text().splitlines():
    try: eventos.append(json.loads(linea))
    except Exception: pass
contadores = {}
for par in os.environ.get("LC_CONT", "").splitlines():
    if "=" in par:
        k, v = par.split("=", 1)
        try: contadores[k] = float(v) if "." in v else int(v)
        except ValueError: contadores[k] = v
contexto = []
# El contexto solo viaja cuando falla: un sobre ok pesa KB, uno con codigo pesa decenas.
if os.environ["LC_STATUS"] == "error":
    for ruta in os.environ.get("LC_CTX", "").split():
        p = pathlib.Path(ruta)
        if p.is_file():
            contexto.append({"archivo": p.name, "bytes": p.stat().st_size,
                             "contenido": p.read_text(errors="replace")})
print(json.dumps({
    "proyecto": os.environ["LC_PROY"], "pipeline": os.environ["LC_PIPE"],
    "run_id": os.environ["LC_RUN"], "status": os.environ["LC_STATUS"],
    "segundos": float(os.environ["LC_SEG"]),
    "inicio": eventos[0]["ts"] if eventos else "",
    "entorno": os.environ.get("ETL_ENTORNO", "local"),
    "eventos": eventos, "contadores": contadores, "contexto": contexto,
}, ensure_ascii=False))
PY
  )"

  if [ -z "$LC_URL" ]; then
    echo "  [debug] envio.omitido: LC_URL sin definir" >&2
    return 0
  fi
  local codigo
  codigo="$(printf '%s' "$sobre" | curl -s -o /dev/null -w '%{http_code}' \
      --max-time "$LC_TIMEOUT" -X POST "$LC_URL" \
      -H 'Content-Type: application/json' \
      ${LC_TOKEN:+-H "Authorization: Bearer $LC_TOKEN"} \
      --data-binary @- 2>/dev/null)" || codigo=000
  if [ "$codigo" = "202" ] || [ "$codigo" = "200" ]; then
    echo "  [info] envio.ok codigo=$codigo" >&2
  else
    # Regla 1: esto NUNCA rompe la corrida. El log local ya quedó escrito.
    echo "  [warn] envio.error codigo=$codigo url=$LC_URL" >&2
  fi
  return 0
}
