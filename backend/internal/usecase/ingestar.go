// Package usecase son los casos de uso: la lógica que no sabe de HTTP ni de dónde se guarda.
// Cada archivo es un caso, leíble de arriba abajo.
package usecase

import (
	"context"
	"log/slog"
	"time"

	"logs-consumer/internal/domain"
)

type Ingesta struct {
	repo  domain.Repositorio
	log   *slog.Logger
	ahora func() time.Time
}

func NuevaIngesta(repo domain.Repositorio, log *slog.Logger) *Ingesta {
	return &Ingesta{repo: repo, log: log, ahora: func() time.Time { return time.Now().UTC() }}
}

// Validar normaliza el sobre y devuelve los avisos SIN guardar nada. Es lo que usa quien está
// conectando un pipeline nuevo: se itera contra este endpoint hasta que no quedan avisos, y
// recién ahí se apunta el emisor al de verdad.
func (i *Ingesta) Validar(bruto []byte) (domain.Corrida, []string, error) {
	return desdeSobre(bruto, i.ahora())
}

// Ingestar guarda. El sobre crudo se escribe primero y entero: es la fuente de verdad, y todo
// lo demás —resumen, estado del pipeline— se puede reconstruir desde él.
func (i *Ingesta) Ingestar(ctx context.Context, bruto []byte) (domain.Corrida, []string, error) {
	c, avisos, err := desdeSobre(bruto, i.ahora())
	if err != nil {
		return c, avisos, err
	}
	if err := i.repo.GuardarCorrida(ctx, c, bruto); err != nil {
		return c, avisos, err
	}
	i.log.Info("corrida.recibida",
		"proyecto", c.Proyecto, "pipeline", c.Pipeline, "run_id", c.RunID,
		"status", string(c.Estado), "segundos", c.Segundos,
		"eventos", len(c.Eventos), "bytes", c.Bytes, "avisos", len(avisos))
	return c, avisos, nil
}

func desdeSobre(bruto []byte, ahora time.Time) (domain.Corrida, []string, error) {
	return domain.DesdeSobre(bruto, ahora)
}
