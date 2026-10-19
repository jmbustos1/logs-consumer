package domain

import "time"

// Pipeline es lo que el consumer sabe de un emisor: su cadencia esperada, cuál de sus eventos
// cuenta una novedad del mundo, y cómo le fue la última vez.
//
// La cadencia la declara quien vigila, no el emisor. Es una decisión de operación: un pipeline
// no sabe cada cuánto "debería" correr, sabe cuándo lo corrieron.
type Pipeline struct {
	Proyecto      string  `json:"proyecto"`
	Pipeline      string  `json:"pipeline"`
	CadenciaH     float64 `json:"cadencia_h"` // 0 = no se vigila el silencio
	EventoNovedad string  `json:"evento_novedad,omitempty"`
	Descripcion   string  `json:"descripcion,omitempty"`

	Ultima      *ResumenCorrida `json:"ultima,omitempty"`
	Corridas    int             `json:"corridas"`
	Actualizado time.Time       `json:"actualizado"`
}

// FactorSilencio: se tolera media cadencia de atraso antes de gritar. Un cron diario que se
// atrasa dos horas no es noticia; uno que lleva 36 horas mudo, sí.
const FactorSilencio = 1.5

func (p Pipeline) EnSilencio(ahora time.Time) bool {
	if p.CadenciaH <= 0 || p.Ultima == nil {
		return false // sin cadencia declarada no se vigila: conectar un emisor nuevo no debe llenar el panel de ámbar
	}
	limite := time.Duration(p.CadenciaH * FactorSilencio * float64(time.Hour))
	return ahora.Sub(p.Ultima.Inicio) > limite
}

// EstadoActual es lo que pinta la portada: el estado de la última corrida, salvo que el
// silencio lo tape — porque una corrida `ok` de hace tres semanas no es una buena noticia.
func (p Pipeline) EstadoActual(ahora time.Time) Estado {
	if p.EnSilencio(ahora) {
		return EstadoEnSilencio
	}
	if p.Ultima == nil {
		return ""
	}
	return p.Ultima.Estado
}

func (p Pipeline) Vencimiento() time.Duration {
	if p.CadenciaH <= 0 {
		return 0
	}
	return time.Duration(p.CadenciaH * FactorSilencio * float64(time.Hour))
}

// Salud es la respuesta de GET /salud: lo único que hay que mirar cuando no se quiere mirar
// todo.
type Salud struct {
	Ahora     time.Time        `json:"ahora"`
	Pipelines []Pipeline       `json:"pipelines"`
	Silencios []Pipeline       `json:"silencios"`
	Fallos    []ResumenCorrida `json:"fallos"`
	Sano      bool             `json:"sano"`
}
