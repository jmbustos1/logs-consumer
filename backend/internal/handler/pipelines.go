package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"logs-consumer/internal/domain"
	"logs-consumer/internal/usecase"
)

type Pipelines struct {
	consulta *usecase.Consulta
	vigia    *usecase.Vigia
	log      *slog.Logger
}

func NuevoPipelines(c *usecase.Consulta, v *usecase.Vigia, log *slog.Logger) *Pipelines {
	return &Pipelines{consulta: c, vigia: v, log: log}
}

// filaPipeline es lo que pinta la portada. El estado calculado va acá y no en el objeto
// guardado porque depende de la hora: un `ok` de hace tres semanas es `en_silencio` hoy.
type filaPipeline struct {
	domain.Pipeline
	EstadoActual domain.Estado `json:"estado_actual"`
	EnSilencio   bool          `json:"en_silencio"`
	DesdeUltima  string        `json:"desde_ultima,omitempty"`
}

func (h *Pipelines) Listar(w http.ResponseWriter, r *http.Request) {
	ps, err := h.consulta.Pipelines(r.Context())
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{"pipelines": filas(ps, time.Now().UTC())})
}

func (h *Pipelines) Detalle(w http.ResponseWriter, r *http.Request) {
	p, err := h.consulta.Pipeline(r.Context(), r.PathValue("proyecto"), r.PathValue("pipeline"))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, filas([]domain.Pipeline{p}, time.Now().UTC())[0])
}

// Declarar fija la cadencia esperada y el evento de novedad. Es lo único que decide el
// operador: el resto lo cuenta el emisor.
func (h *Pipelines) Declarar(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		CadenciaH     float64 `json:"cadencia_h"`
		EventoNovedad string  `json:"evento_novedad"`
		Descripcion   string  `json:"descripcion"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&cuerpo); err != nil {
		escribirError(w, h.log, fmt.Errorf("%w: %v", domain.ErrInvalido, err))
		return
	}
	p, err := h.consulta.DeclararPipeline(r.Context(), domain.Pipeline{
		Proyecto:      r.PathValue("proyecto"),
		Pipeline:      r.PathValue("pipeline"),
		CadenciaH:     cuerpo.CadenciaH,
		EventoNovedad: cuerpo.EventoNovedad,
		Descripcion:   cuerpo.Descripcion,
	})
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, filas([]domain.Pipeline{p}, time.Now().UTC())[0])
}

func (h *Pipelines) Eliminar(w http.ResponseWriter, r *http.Request) {
	proyecto, pipeline := r.PathValue("proyecto"), r.PathValue("pipeline")
	b, err := h.consulta.Eliminar(r.Context(), proyecto, pipeline, r.URL.Query().Get("confirmar"))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	// Queda en el log del servicio: lo borrado no se puede auditar después mirando los datos,
	// porque justamente ya no están.
	h.log.Warn("pipeline.eliminado", "proyecto", proyecto, "pipeline", pipeline,
		"sobres", b.Sobres, "resumenes", b.Resumenes, "estado", b.Estado)
	escribirJSON(w, http.StatusOK, map[string]any{
		"eliminado": proyecto + "/" + pipeline,
		"objetos":   b,
		"total":     b.Total(),
	})
}

func (h *Pipelines) Salud(w http.ResponseWriter, r *http.Request) {
	s, err := h.vigia.Salud(r.Context())
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, s)
}

func filas(ps []domain.Pipeline, ahora time.Time) []filaPipeline {
	out := make([]filaPipeline, 0, len(ps))
	for _, p := range ps {
		f := filaPipeline{Pipeline: p, EstadoActual: p.EstadoActual(ahora), EnSilencio: p.EnSilencio(ahora)}
		if p.Ultima != nil {
			f.DesdeUltima = ahora.Sub(p.Ultima.Inicio).Round(time.Minute).String()
		}
		out = append(out, f)
	}
	return out
}
