// Package notificador es el enchufe de alertas. No hay canal elegido todavía: esta
// implementación no manda nada y lo dice en el log.
//
// Está acá para que el día que exista un canal —correo, Telegram, lo que sea— agregarlo sea
// escribir un tipo que cumpla domain.Notificador, no reescribir el vigía. Misma idea que el
// envio.py de los pipelines, que existió meses antes de que existiera este servicio.
package notificador

import (
	"context"
	"log/slog"
)

type Silencioso struct{ log *slog.Logger }

func NuevoSilencioso(log *slog.Logger) *Silencioso { return &Silencioso{log: log} }

func (s *Silencioso) Avisar(ctx context.Context, asunto, cuerpo string) error {
	// Deliberadamente no devuelve error: un canal de alertas que no existe no es un fallo
	// del servicio. Queda la línea en el log para poder auditar qué se habría avisado.
	s.log.Info("alerta.no_enviada", "motivo", "sin canal configurado", "asunto", asunto)
	return nil
}
