"""Logging por corrida — un .jsonl por ejecución, una línea JSON por evento.

Calcado del molde de lids-searcher. Sigue siendo texto plano —se abre con `cat`, se baja con
`scp`— pero se puede filtrar, que es la diferencia entre encontrar el problema y leer mil
líneas a ojo.
"""
from __future__ import annotations

import json
import os
import platform
import random
import sys
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path

from . import config

_LEVELS = {"debug": 10, "info": 20, "warn": 30, "error": 40}


def nuevo_run_id() -> str:
    return (datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
            + f"-{random.randint(0, 0xFFF):03x}")


class Run:
    def __init__(self, comando: str, echo: str = "info", log_dir: Path | None = None):
        self.run_id = nuevo_run_id()
        self.comando = comando
        self.inicio_mono = time.monotonic()
        self.inicio_utc = datetime.now(timezone.utc)
        self.echo = _LEVELS.get(echo, 20)
        self.contadores: dict[str, int] = {}
        self.eventos: list[dict] = []
        # En Fargate, CloudWatch ya captura stdout. Espejar cada evento cuesta un print()
        # y evita montar un volumen solo para depurar.
        self.json_stdout = os.environ.get("ETL_LOG_JSON_STDOUT", "") not in ("", "0")
        d = log_dir or config.LOG_DIR
        d.mkdir(parents=True, exist_ok=True)
        self.path = d / f"{self.run_id}.jsonl"
        self._fh = self.path.open("a", encoding="utf-8")
        self.event("run.start", comando=comando,
                   proyecto=config.PROYECTO, pipeline=config.PIPELINE,
                   entorno=config.entorno(), version=config.version(),
                   host=platform.node(), pid=os.getpid(),
                   python=platform.python_version())

    def event(self, ev: str, level: str = "info", **campos) -> None:
        rec = {"ts": datetime.now(timezone.utc).isoformat(timespec="milliseconds"),
               "run": self.run_id, "lvl": level, "ev": ev,
               **{k: v for k, v in campos.items() if v is not None}}
        linea = json.dumps(rec, ensure_ascii=False, default=str)
        # Tolerante a que el archivo ya esté cerrado: un evento tardío —por ejemplo el
        # `stage.end` de un context manager del que se salió con return— no debe tumbar
        # la corrida. Se pierde la línea en disco, no el resultado.
        if not self._fh.closed:
            self._fh.write(linea + "\n")
            self._fh.flush()      # barato, y una corrida muerta igual deja su log
        self.eventos.append(rec)
        if self.json_stdout:
            print(linea, flush=True)
        if _LEVELS.get(level, 20) >= self.echo:
            extra = " ".join(f"{k}={v}" for k, v in campos.items())
            print(f"  [{level}] {ev} {extra}".rstrip(), file=sys.stderr)

    def bump(self, nombre: str, n: int = 1) -> None:
        self.contadores[nombre] = self.contadores.get(nombre, 0) + n

    @contextmanager
    def etapa(self, nombre: str, **campos):
        t0 = time.monotonic()
        self.event("stage.start", stage=nombre, **campos)
        try:
            yield self
        except Exception as exc:
            self.event("stage.error", level="error", stage=nombre,
                       error=f"{type(exc).__name__}: {exc}",
                       ms=round((time.monotonic() - t0) * 1000, 1))
            raise
        else:
            self.event("stage.end", stage=nombre,
                       ms=round((time.monotonic() - t0) * 1000, 1))

    # ── la corrida que "funciona" pero no hizo nada ──────────────────────────
    # Heredado de lids, que lo aprendió a los golpes: una corrida que termina en `ok` sin
    # haber hecho nada es PEOR que una que falla, porque la que falla se nota.
    def _revisar_actividad(self) -> str | None:
        c = self.contadores
        if c.get("descarga.bytes", 0) == 0:
            return "no se descargó ni un byte"
        if c.get("version.nueva") and c.get("diff.nuevos", 0) == 0 \
                and c.get("diff.salidos", 0) == 0 and c.get("diff.cambios_estado", 0) == 0:
            return ("el servidor publicó una versión nueva pero el contenido es idéntico al "
                    "anterior: puede ser caché rancia río arriba")
        return None

    def resumen(self, status: str) -> dict:
        """El sobre que va al dashboard. Ver plan.md § etiqueta de proyecto."""
        return {"proyecto": config.PROYECTO, "pipeline": config.PIPELINE,
                "entorno": config.entorno(), "version": config.version(),
                "run_id": self.run_id, "host": platform.node(), "comando": self.comando,
                "status": status, "inicio": self.inicio_utc.isoformat(timespec="seconds"),
                "segundos": round(time.monotonic() - self.inicio_mono, 1),
                "contadores": dict(self.contadores), "eventos": self.eventos,
                # Qué hace falta para diagnosticar esto sin conocer el proyecto. El contenido
                # viaja solo si la corrida falló (ver config.CONTEXTO_INLINE).
                "contexto": config.contexto(
                    config.CONTEXTO_INLINE == "siempre"
                    or (config.CONTEXTO_INLINE == "error" and status == "error"))}

    def close(self, status: str = "ok") -> str:
        if self._fh.closed:       # idempotente: cerrar dos veces no es un error
            return status
        if status == "ok" and (motivo := self._revisar_actividad()):
            self.event("run.inactiva", level="warn", motivo=motivo)
            status = "sin_novedad"
        self.event("run.end", status=status,
                   segundos=round(time.monotonic() - self.inicio_mono, 1),
                   **self.contadores)
        self._fh.close()
        return status
