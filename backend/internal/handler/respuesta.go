// Package handler traduce HTTP a casos de uso y de vuelta. No decide nada: si acá aparece una
// regla de negocio, está en el lugar equivocado.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"logs-consumer/internal/domain"
)

func escribirJSON(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func escribirTexto(w http.ResponseWriter, codigo int, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(codigo)
	_, _ = w.Write([]byte(s))
}

type errorJSON struct {
	Error   string   `json:"error"`
	Detalle string   `json:"detalle,omitempty"`
	Avisos  []string `json:"avisos,omitempty"`
}

// escribirError es el ÚNICO lugar donde un error de dominio se convierte en un código HTTP.
// Los casos de uso no saben qué es un 404 y no tienen por qué saberlo.
func escribirError(w http.ResponseWriter, log *slog.Logger, err error) {
	codigo := http.StatusInternalServerError
	mensaje := "error interno"

	switch {
	case errors.Is(err, domain.ErrInvalido):
		codigo, mensaje = http.StatusBadRequest, "sobre inválido"
	case errors.Is(err, domain.ErrNoEncontrado):
		codigo, mensaje = http.StatusNotFound, "no encontrado"
	case errors.Is(err, domain.ErrDemasiadoGrande):
		codigo, mensaje = http.StatusRequestEntityTooLarge, "cuerpo demasiado grande"
	case errors.Is(err, domain.ErrNoAutorizado):
		codigo, mensaje = http.StatusUnauthorized, "no autorizado"
	case errors.Is(err, domain.ErrDesactivado):
		codigo, mensaje = http.StatusNotImplemented, "funcionalidad desactivada"
	default:
		// Un 500 es un bug nuestro: se registra entero. Los 4xx son del que llama y su
		// detalle ya viaja en la respuesta.
		if log != nil {
			log.Error("http.error_interno", "error", err.Error())
		}
	}
	escribirJSON(w, codigo, errorJSON{Error: mensaje, Detalle: err.Error()})
}
