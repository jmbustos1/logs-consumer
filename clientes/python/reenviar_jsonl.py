#!/usr/bin/env python3
"""Reenvía al consumer corridas que ya ocurrieron y solo existen como .jsonl en disco.

Para qué sirve: enchufar el consumer a un pipeline que lleva meses corriendo no debería
empezar con el panel vacío. Este script reconstruye el sobre a partir del log de la corrida
—que es la fuente de verdad del emisor— y lo manda.

Lo que NO puede recuperar es el campo `contexto`: el código del pipeline se adjunta en el
momento del fallo, no después. Las corridas reenviadas van a tener bundle sin código.

    python3 reenviar_jsonl.py --url http://localhost:8787/api/v1/corridas ~/proyecto/logs/*.jsonl
"""
from __future__ import annotations

import argparse
import glob
import json
import pathlib
import sys
import urllib.error
import urllib.request

CAMPOS_RUN_START = ("proyecto", "pipeline", "entorno", "version", "host", "comando")


def sobre_desde_jsonl(ruta: pathlib.Path, proyecto: str | None, pipeline: str | None) -> dict | None:
    eventos = []
    for linea in ruta.read_text(encoding="utf-8").splitlines():
        linea = linea.strip()
        if not linea:
            continue
        try:
            eventos.append(json.loads(linea))
        except json.JSONDecodeError:
            continue          # una línea rota no invalida la corrida entera
    if not eventos:
        return None

    inicio = next((e for e in eventos if e.get("ev") == "run.start"), {})
    fin = next((e for e in reversed(eventos) if e.get("ev") == "run.end"), {})

    sobre = {
        "run_id": inicio.get("run") or eventos[0].get("run") or ruta.stem,
        "proyecto": proyecto or inicio.get("proyecto", ""),
        "pipeline": pipeline or inicio.get("pipeline", ""),
        "status": fin.get("status", "ok" if fin else "error"),
        "inicio": inicio.get("ts") or eventos[0].get("ts", ""),
        "segundos": fin.get("segundos", 0),
        "eventos": eventos,
        # Los contadores viajan como campos sueltos de run.end; se separan de lo que es
        # metadato de la corrida.
        "contadores": {k: v for k, v in fin.items()
                       if k not in ("ts", "run", "lvl", "ev", "status", "segundos")},
    }
    for c in CAMPOS_RUN_START:
        if c not in ("proyecto", "pipeline") and inicio.get(c):
            sobre[c] = inicio[c]
    return sobre


def enviar(url: str, token: str, sobre: dict, timeout: float) -> tuple[int, str]:
    cuerpo = json.dumps(sobre, ensure_ascii=False, default=str).encode("utf-8")
    req = urllib.request.Request(url, data=cuerpo, method="POST")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.read().decode("utf-8", "replace")[:300]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")[:300]
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("logs", nargs="+", help="archivos .jsonl (admite comodines)")
    ap.add_argument("--url", required=True)
    ap.add_argument("--token", default="")
    ap.add_argument("--proyecto", default=None, help="fuerza el proyecto si el log no lo trae")
    ap.add_argument("--pipeline", default=None)
    ap.add_argument("--timeout", type=float, default=15.0)
    ap.add_argument("--simular", action="store_true", help="arma los sobres y no envía nada")
    a = ap.parse_args()

    rutas = sorted({pathlib.Path(p) for patron in a.logs for p in glob.glob(patron)})
    ok = fallidos = vacios = 0
    for ruta in rutas:
        sobre = sobre_desde_jsonl(ruta, a.proyecto, a.pipeline)
        if not sobre or not sobre["proyecto"] or not sobre["pipeline"]:
            print(f"  omitido  {ruta.name}: sin run.start utilizable (probá --proyecto/--pipeline)")
            vacios += 1
            continue
        if a.simular:
            print(f"  {sobre['run_id']}  {sobre['proyecto']}/{sobre['pipeline']:16} "
                  f"{sobre['status']:12} {len(sobre['eventos']):4} eventos")
            ok += 1
            continue
        codigo, cuerpo = enviar(a.url, a.token, sobre, a.timeout)
        if 200 <= codigo < 300:
            ok += 1
            print(f"  {codigo}  {sobre['run_id']}  {sobre['pipeline']:16} {sobre['status']}")
        else:
            fallidos += 1
            print(f"  {codigo}  {sobre['run_id']}  {cuerpo}")

    print(f"\n  {ok} enviadas · {fallidos} fallidas · {vacios} omitidas")
    return 1 if fallidos else 0


if __name__ == "__main__":
    sys.exit(main())
