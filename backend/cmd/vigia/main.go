// El vigía como ejecutable suelto: mira quién dejó de hablar y avisa.
//
// Es el mismo caso de uso que sirve GET /api/v1/salud. Existe aparte porque un cron no
// debería depender de que el panel esté arriba, y porque el día que haya canal de alerta
// esto es lo que va a correr cada mañana.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"logs-consumer/config"
	"logs-consumer/internal/adapter/almacen"
	"logs-consumer/internal/adapter/almacen/disco"
	"logs-consumer/internal/adapter/notificador"
	"logs-consumer/internal/logjson"
	"logs-consumer/internal/usecase"
)

func main() {
	cfg := config.Cargar()
	log := logjson.Nuevo(cfg.Entorno, cfg.Version)

	alm, err := disco.Nuevo(cfg.DatosDir)
	if err != nil {
		log.Error("vigia.sin_almacen", "error", err)
		os.Exit(1)
	}
	repo := almacen.NuevoRepo(alm)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := usecase.NuevoVigia(repo).Salud(ctx)
	if err != nil {
		log.Error("vigia.fallo", "error", err)
		os.Exit(1)
	}

	for _, p := range s.Silencios {
		log.Warn("pipeline.en_silencio", "proyecto", p.Proyecto, "pipeline", p.Pipeline,
			"cadencia_h", p.CadenciaH, "ultima", p.Ultima.Inicio)
	}
	for _, f := range s.Fallos {
		log.Warn("pipeline.ultimo_fallo", "proyecto", f.Proyecto, "pipeline", f.Pipeline,
			"run_id", f.RunID, "etapa", f.EtapaFallo, "error", f.Error)
	}

	if !s.Sano {
		aviso := notificador.NuevoSilencioso(log)
		_ = aviso.Avisar(ctx, "pipelines con problemas",
			fmt.Sprintf("%d en silencio, %d con la última corrida fallida", len(s.Silencios), len(s.Fallos)))
		// Código 1 para que un cron lo note aunque nadie mire el log.
		os.Exit(1)
	}
	log.Info("vigia.ok", "pipelines", len(s.Pipelines))
}
