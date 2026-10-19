package usecase

import (
	"context"
	"time"

	"logs-consumer/internal/domain"
)

// Vigia responde la pregunta que ningún emisor puede responder: ¿cuál de mis pipelines dejó
// de hablar? Un pipeline que falla manda su sobre y se nota. Uno que muere duro —OOM, tarea
// matada, cron desactivado— no manda nada, y el silencio es la única señal que queda.
type Vigia struct {
	repo  domain.Repositorio
	ahora func() time.Time
}

func NuevoVigia(repo domain.Repositorio) *Vigia {
	return &Vigia{repo: repo, ahora: func() time.Time { return time.Now().UTC() }}
}

func (v *Vigia) Salud(ctx context.Context) (domain.Salud, error) {
	ahora := v.ahora()
	pipes, err := v.repo.ListarPipelines(ctx)
	if err != nil {
		return domain.Salud{}, err
	}

	// Listas vacías y no nil: "no hay silencios" y "no sé" no pueden verse igual en el JSON.
	if pipes == nil {
		pipes = []domain.Pipeline{}
	}
	s := domain.Salud{
		Ahora: ahora, Pipelines: pipes, Sano: true,
		Silencios: []domain.Pipeline{}, Fallos: []domain.ResumenCorrida{},
	}
	for _, p := range pipes {
		if p.EnSilencio(ahora) {
			s.Silencios = append(s.Silencios, p)
			s.Sano = false
		}
		if p.Ultima != nil && p.Ultima.Estado == domain.EstadoError {
			s.Fallos = append(s.Fallos, *p.Ultima)
			s.Sano = false
		}
	}
	domain.OrdenarPorInicio(s.Fallos)
	return s, nil
}
