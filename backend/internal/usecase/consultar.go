package usecase

import (
	"context"
	"fmt"
	"strings"

	"logs-consumer/internal/domain"
)

type Consulta struct{ repo domain.Repositorio }

func NuevaConsulta(repo domain.Repositorio) *Consulta { return &Consulta{repo: repo} }

func (c *Consulta) Listar(ctx context.Context, f domain.Filtro) ([]domain.ResumenCorrida, error) {
	rs, err := c.repo.ListarCorridas(ctx, f)
	if err != nil {
		return nil, err
	}
	domain.OrdenarPorInicio(rs)
	return rs, nil
}

// Resolver acepta la referencia completa (rápida) o solo el run_id (busca). El panel navega
// desde un listado, así que casi siempre trae las tres partes.
func (c *Consulta) Resolver(ctx context.Context, ref domain.Ref) (domain.Ref, error) {
	if ref.Completa() {
		return ref, nil
	}
	if ref.RunID == "" {
		return ref, fmt.Errorf("%w: falta el run_id", domain.ErrInvalido)
	}
	return c.repo.BuscarRun(ctx, ref.RunID)
}

func (c *Consulta) Corrida(ctx context.Context, ref domain.Ref) (domain.Corrida, error) {
	r, err := c.Resolver(ctx, ref)
	if err != nil {
		return domain.Corrida{}, err
	}
	return c.repo.LeerCorrida(ctx, r)
}

// VistaCorrida es lo que consume la pantalla principal: la corrida reconstruida en etapas,
// con su salida cruda, más el pipeline al que pertenece para poder mostrar la novedad.
type VistaCorrida struct {
	Corrida domain.Corrida  `json:"corrida"`
	Etapas  []domain.Etapa  `json:"etapas"`
	Novedad *domain.Evento  `json:"novedad,omitempty"`
	Pipe    domain.Pipeline `json:"pipeline"`
}

func (c *Consulta) Vista(ctx context.Context, ref domain.Ref, nivelMin string, etapa string) (VistaCorrida, error) {
	corrida, err := c.Corrida(ctx, ref)
	if err != nil {
		return VistaCorrida{}, err
	}
	v := VistaCorrida{Corrida: corrida}

	if p, err := c.repo.LeerPipeline(ctx, corrida.Proyecto, corrida.Pipeline); err == nil {
		v.Pipe = p
		if ev, ok := corrida.Novedad(p.EventoNovedad); ok {
			v.Novedad = &ev
		}
	}

	v.Etapas = filtrarEtapas(corrida.Etapas(), nivelMin, etapa)
	// La lista plana de eventos ya vive dentro de las etapas; devolverla dos veces sería
	// duplicar el 90 % del peso de la respuesta.
	v.Corrida.Eventos = nil
	return v, nil
}

func filtrarEtapas(etapas []domain.Etapa, nivelMin, soloEtapa string) []domain.Etapa {
	peso := 0
	if nivelMin != "" {
		peso = domain.PesoNivel(nivelMin)
	}
	out := make([]domain.Etapa, 0, len(etapas))
	for _, e := range etapas {
		if soloEtapa != "" && !strings.EqualFold(e.Nombre, soloEtapa) {
			continue
		}
		if peso > 0 {
			evs := make([]domain.Evento, 0, len(e.Eventos))
			for _, ev := range e.Eventos {
				if domain.PesoNivel(ev.Nivel) >= peso {
					evs = append(evs, ev)
				}
			}
			e.Eventos = evs
		}
		out = append(out, e)
	}
	return out
}

// Eliminar borra un pipeline entero. Exige que se repita su nombre en `confirmar`, como hace
// `terraform destroy`: es una operación que no se deshace, y un curl mal copiado no debería
// poder vaciar la historia de un pipeline sano.
func (c *Consulta) Eliminar(ctx context.Context, proyecto, pipeline, confirmar string) (domain.Borrado, error) {
	if confirmar != pipeline {
		return domain.Borrado{}, fmt.Errorf(
			"%w: para borrar hay que repetir el nombre en `confirmar` (se esperaba %q y llegó %q)",
			domain.ErrInvalido, pipeline, confirmar)
	}
	if _, err := c.repo.LeerPipeline(ctx, proyecto, pipeline); err != nil {
		return domain.Borrado{}, fmt.Errorf("%w: no hay un pipeline %s/%s", domain.ErrNoEncontrado, proyecto, pipeline)
	}
	return c.repo.EliminarPipeline(ctx, proyecto, pipeline)
}

func (c *Consulta) Pipelines(ctx context.Context) ([]domain.Pipeline, error) {
	return c.repo.ListarPipelines(ctx)
}

func (c *Consulta) Pipeline(ctx context.Context, proyecto, pipeline string) (domain.Pipeline, error) {
	return c.repo.LeerPipeline(ctx, proyecto, pipeline)
}

// DeclararPipeline fija la cadencia esperada y el evento de novedad. Es lo único que el
// operador decide sobre un pipeline; lo demás lo cuenta el emisor.
func (c *Consulta) DeclararPipeline(ctx context.Context, p domain.Pipeline) (domain.Pipeline, error) {
	actual, err := c.repo.LeerPipeline(ctx, p.Proyecto, p.Pipeline)
	if err == nil {
		// No se pisa lo que reportaron los emisores con lo que manda un PUT.
		actual.CadenciaH = p.CadenciaH
		actual.EventoNovedad = p.EventoNovedad
		if p.Descripcion != "" {
			actual.Descripcion = p.Descripcion
		}
		p = actual
	}
	if err := c.repo.GuardarPipeline(ctx, p); err != nil {
		return domain.Pipeline{}, err
	}
	return p, nil
}
