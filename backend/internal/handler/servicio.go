package handler

import (
	_ "embed"
	"net/http"
	"time"
)

// El contrato viaja con el binario. Un agente que está conectando un pipeline nuevo puede
// leerlo desde la API sin tener este repositorio a mano — que es exactamente la situación en
// la que va a estar.
//
//go:embed contrato.md
var contrato string

type Servicio struct {
	Version string
	Entorno string
	Arranco time.Time
}

func (s *Servicio) Check(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// Health mira lo que el servicio necesita para funcionar. Hoy solo el reloj y su propio
// almacén; se distingue de Check porque un balanceador pregunta "¿estás vivo?" y un humano
// pregunta "¿estás sano?", y no son la misma pregunta.
func (s *Servicio) Health(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.Version,
		"entorno": s.Entorno,
		"arriba":  time.Since(s.Arranco).Round(time.Second).String(),
	})
}

func (s *Servicio) Contrato(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(contrato))
}
