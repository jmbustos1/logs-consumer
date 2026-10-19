// Package router: todas las rutas del servicio en una pantalla, con lo que protege a cada
// una al lado. Si hay que preguntarse "¿esto pide token?", la respuesta se lee acá.
package router

import (
	"log/slog"
	"net/http"

	"logs-consumer/config"
	"logs-consumer/internal/handler"
	"logs-consumer/internal/middleware"
)

type Deps struct {
	Cfg       config.Config
	Log       *slog.Logger
	Corridas  *handler.Corridas
	Pipelines *handler.Pipelines
	Servicio  *handler.Servicio
}

func Nuevo(d Deps) http.Handler {
	mux := http.NewServeMux()

	// El emisor escribe. Es el token que los pipelines ponen en ETL_LOGS_TOKEN.
	emisor := middleware.RequiereToken(d.Cfg.TokenEmisor, "emisor")
	// El lector lee. Es el del panel.
	lector := middleware.RequiereToken(d.Cfg.TokenLector, "lector")

	ruta := func(patron string, h http.HandlerFunc, cs ...middleware.Cadena) {
		mux.Handle(patron, middleware.Componer(h, cs...))
	}

	// ── ingesta ──────────────────────────────────────────────────────────────
	ruta("POST /api/v1/corridas", d.Corridas.Ingestar, emisor)
	ruta("POST /api/v1/corridas/validar", d.Corridas.Validar, emisor)

	// ── consulta ─────────────────────────────────────────────────────────────
	ruta("GET /api/v1/corridas", d.Corridas.Listar, lector)
	ruta("GET /api/v1/corridas/{run_id}", d.Corridas.Detalle, lector)
	ruta("GET /api/v1/corridas/{run_id}/vista", d.Corridas.Vista, lector)
	ruta("GET /api/v1/corridas/{run_id}/bundle", d.Corridas.Bundle, lector)
	ruta("POST /api/v1/corridas/{run_id}/diagnostico", d.Corridas.Diagnosticar, lector)

	ruta("GET /api/v1/pipelines", d.Pipelines.Listar, lector)
	ruta("GET /api/v1/pipelines/{proyecto}/{pipeline}", d.Pipelines.Detalle, lector)
	ruta("PUT /api/v1/pipelines/{proyecto}/{pipeline}", d.Pipelines.Declarar, lector)
	// Borrar pide el token de ESCRITURA, no el de lectura. Así el panel —que solo tiene el
	// del lector— no puede destruir nada, y el token que vive en tu navegador es inofensivo.
	ruta("DELETE /api/v1/pipelines/{proyecto}/{pipeline}", d.Pipelines.Eliminar, emisor)
	ruta("GET /api/v1/salud", d.Pipelines.Salud, lector)

	// ── sin token, a propósito ───────────────────────────────────────────────
	// El contrato es público: quien está conectando un pipeline todavía no tiene token, y
	// pedírselo para leer la documentación sería un acertijo. No expone ningún dato.
	ruta("GET /api/v1/contrato", d.Servicio.Contrato)
	ruta("GET /check", d.Servicio.Check)
	ruta("GET /health", d.Servicio.Health)

	return middleware.Componer(mux,
		middleware.Recuperar(d.Log),
		middleware.Registrar(d.Log),
		middleware.CORS(d.Cfg.OrigenesCORS),
	)
}
