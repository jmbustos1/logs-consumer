"""Despacho del resumen de la corrida al dashboard de pipelines.

PLACEHOLDER — el dashboard es un proyecto aparte que todavía no existe. Acá solo está el
enchufe: se define ETL_LOGS_URL y empieza a llegar, sin tocar el ETL.

Tres reglas que no se negocian:

1. Nunca puede romper el ETL. Si el destino está caído, tiene el certificado vencido o
   simplemente no existe, la corrida sigue y termina bien. Esto es telemetría, no el trabajo.
2. Un solo POST al final, con el resumen completo. Ocho etapas una vez al mes no justifican
   un request por evento: solo multiplicaría los modos de falla.
3. El .jsonl local es la fuente de verdad. Esto es una copia.
"""
from __future__ import annotations

import json
import urllib.error
import urllib.request

from . import config


def despachar(resumen: dict, log=None) -> bool:
    """Devuelve True si se envió, False si no había destino o si falló.

    Nunca lanza: el fallo se registra en el log local y la corrida continúa.
    """
    if not config.LOGS_URL:
        if log:
            log.event("envio.omitido", level="debug", motivo="ETL_LOGS_URL sin definir")
        return False

    cuerpo = json.dumps(resumen, ensure_ascii=False, default=str).encode("utf-8")
    req = urllib.request.Request(config.LOGS_URL, data=cuerpo, method="POST")
    req.add_header("Content-Type", "application/json")
    if config.LOGS_TOKEN:
        req.add_header("Authorization", f"Bearer {config.LOGS_TOKEN}")

    try:
        with urllib.request.urlopen(req, timeout=config.LOGS_TIMEOUT_S) as r:
            if log:
                log.event("envio.ok", codigo=r.status, bytes=len(cuerpo))
            return True
    except Exception as exc:
        # Deliberadamente amplio. Cualquier cosa que pase acá —DNS, TLS, timeout, 500,
        # el dashboard a medio construir— es irrelevante para el resultado del ETL.
        if log:
            log.event("envio.error", level="warn",
                      error=f"{type(exc).__name__}: {exc}", url=config.LOGS_URL[:120])
        return False
