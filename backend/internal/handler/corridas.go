package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"logs-consumer/internal/domain"
	"logs-consumer/internal/usecase"
)

type Corridas struct {
	ingesta  *usecase.Ingesta
	consulta *usecase.Consulta
	diag     *usecase.Diagnostico
	maxBytes int64
	log      *slog.Logger
}

func NuevoCorridas(i *usecase.Ingesta, c *usecase.Consulta, d *usecase.Diagnostico, maxBytes int64, log *slog.Logger) *Corridas {
	return &Corridas{ingesta: i, consulta: c, diag: d, maxBytes: maxBytes, log: log}
}

type respIngesta struct {
	RunID    string   `json:"run_id"`
	Proyecto string   `json:"proyecto"`
	Pipeline string   `json:"pipeline"`
	Estado   string   `json:"status"`
	Avisos   []string `json:"avisos,omitempty"`
}

// Ingestar: POST /api/v1/corridas. Devuelve 202 — el emisor no espera nada de nosotros, y su
// envio.py trata cualquier respuesta que no sea 2xx como un warn que no rompe su corrida.
func (h *Corridas) Ingestar(w http.ResponseWriter, r *http.Request) {
	bruto, err := h.leerCuerpo(w, r)
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	c, avisos, err := h.ingesta.Ingestar(r.Context(), bruto)
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusAccepted, respIngesta{
		RunID: c.RunID, Proyecto: c.Proyecto, Pipeline: c.Pipeline,
		Estado: string(c.Estado), Avisos: avisos,
	})
}

// Validar: POST /api/v1/corridas/validar. No guarda nada. Es el endpoint contra el que itera
// quien está conectando un pipeline nuevo hasta que no quedan avisos.
func (h *Corridas) Validar(w http.ResponseWriter, r *http.Request) {
	bruto, err := h.leerCuerpo(w, r)
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	c, avisos, err := h.ingesta.Validar(bruto)
	if err != nil {
		escribirJSON(w, http.StatusBadRequest, errorJSON{
			Error: "sobre inválido", Detalle: err.Error(), Avisos: avisos,
		})
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{
		"valido": true,
		"avisos": avisos,
		"leido": respIngesta{
			RunID: c.RunID, Proyecto: c.Proyecto, Pipeline: c.Pipeline, Estado: string(c.Estado),
		},
		"etapas_detectadas": len(c.Etapas()),
		"eventos":           len(c.Eventos),
	})
}

func (h *Corridas) leerCuerpo(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	// Rechazar por Content-Length antes de leer un solo byte. Si se espera a que
	// MaxBytesReader corte, la respuesta sale cuando el emisor todavía está subiendo y lo que
	// ve del otro lado es una conexión reseteada en vez de un 413 legible.
	if r.ContentLength > h.maxBytes {
		return nil, fmt.Errorf("%w: el sobre declara %d bytes y el límite es %d",
			domain.ErrDemasiadoGrande, r.ContentLength, h.maxBytes)
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	bruto, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, fmt.Errorf("%w: el sobre supera %d bytes", domain.ErrDemasiadoGrande, h.maxBytes)
		}
		return nil, fmt.Errorf("%w: no se pudo leer el cuerpo: %v", domain.ErrInvalido, err)
	}
	if len(bruto) == 0 {
		return nil, fmt.Errorf("%w: cuerpo vacío", domain.ErrInvalido)
	}
	return bruto, nil
}

func (h *Corridas) Listar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limite, _ := strconv.Atoi(q.Get("limite"))
	f := domain.Filtro{
		Proyecto: q.Get("proyecto"),
		Pipeline: q.Get("pipeline"),
		Estado:   domain.Estado(q.Get("status")),
		Desde:    q.Get("desde"),
		Hasta:    q.Get("hasta"),
		Limite:   limite,
	}
	rs, err := h.consulta.Listar(r.Context(), f)
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	if rs == nil {
		rs = []domain.ResumenCorrida{}
	}
	escribirJSON(w, http.StatusOK, map[string]any{"corridas": rs, "total": len(rs)})
}

func (h *Corridas) Detalle(w http.ResponseWriter, r *http.Request) {
	c, err := h.consulta.Corrida(r.Context(), refDe(r))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, c)
}

// Vista es lo que consume la pantalla principal: la corrida ya reconstruida en etapas, con la
// salida cruda de cada una. El front no tiene que saber cómo se arman las etapas a partir de
// los eventos; eso es conocimiento del contrato y vive del lado del servidor.
func (h *Corridas) Vista(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, err := h.consulta.Vista(r.Context(), refDe(r), q.Get("nivel"), q.Get("etapa"))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirJSON(w, http.StatusOK, v)
}

func (h *Corridas) Bundle(w http.ResponseWriter, r *http.Request) {
	ref, err := h.consulta.Resolver(r.Context(), refDe(r))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	texto, err := h.diag.Bundle(r.Context(), ref)
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	escribirTexto(w, http.StatusOK, texto)
}

func (h *Corridas) Diagnosticar(w http.ResponseWriter, r *http.Request) {
	ref, err := h.consulta.Resolver(r.Context(), refDe(r))
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	p, err := h.diag.Proponer(r.Context(), ref)
	if errors.Is(err, domain.ErrDesactivado) {
		escribirJSON(w, http.StatusNotImplemented, errorJSON{
			Error: "diagnóstico desactivado",
			Detalle: "Para encenderlo: LC_DIAGNOSTICO=on y ANTHROPIC_API_KEY definida. " +
				"Mientras tanto, GET .../bundle devuelve el mismo contexto para pegar a mano.",
		})
		return
	}
	if err != nil {
		escribirError(w, h.log, err)
		return
	}
	h.log.Info("diagnostico.emitido", "run_id", ref.RunID, "modelo", p.Modelo,
		"tokens_entrada", p.TokensEnt, "tokens_salida", p.TokensSal, "costo_usd", p.CostoUSD)
	escribirJSON(w, http.StatusOK, p)
}

// refDe acepta el run_id de la ruta y, si vienen, proyecto y pipeline como query: con las tres
// partes se va derecho al objeto; con el run_id solo hay que buscarlo.
func refDe(r *http.Request) domain.Ref {
	return domain.Ref{
		Proyecto: r.URL.Query().Get("proyecto"),
		Pipeline: r.URL.Query().Get("pipeline"),
		RunID:    r.PathValue("run_id"),
	}
}
