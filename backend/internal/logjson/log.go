// Package logjson es el logging del propio consumer: una línea JSON por evento, a stdout.
//
// Mismo criterio que los pipelines que vigila —texto plano, una línea por evento, filtrable
// con grep— porque en Lambda y en Fargate stdout ya lo captura CloudWatch y montar cualquier
// otra cosa sería trabajo para el mismo resultado.
//
// El consumer NO se vigila a sí mismo con el consumer: eso sería una serpiente comiéndose la
// cola. Si se cae, lo que lo delata es que el panel no responde.
package logjson

import (
	"log/slog"
	"os"
)

func Nuevo(entorno, version string) *slog.Logger {
	nivel := slog.LevelInfo
	if os.Getenv("LC_LOG_DEBUG") != "" {
		nivel = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})
	return slog.New(h).With("servicio", "logs-consumer", "entorno", entorno, "version", version)
}
