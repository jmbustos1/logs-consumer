package domain

import (
	"testing"
	"time"
)

func pipeConUltima(cadencia float64, hace time.Duration) Pipeline {
	u := ResumenCorrida{Estado: EstadoOK, Inicio: ahora.Add(-hace)}
	return Pipeline{Proyecto: "p", Pipeline: "q", CadenciaH: cadencia, Ultima: &u}
}

func TestSilencioSoloConCadenciaDeclarada(t *testing.T) {
	// Un pipeline que reporta pero no declaró cadencia NO se vigila: conectar un emisor nuevo
	// no puede llenar el panel de alarmas.
	sinCadencia := pipeConUltima(0, 500*time.Hour)
	if sinCadencia.EnSilencio(ahora) {
		t.Error("sin cadencia declarada no debería vigilarse el silencio")
	}

	// Sin corridas tampoco: no se puede estar en silencio si nunca se habló.
	nueva := Pipeline{CadenciaH: 24}
	if nueva.EnSilencio(ahora) {
		t.Error("un pipeline sin corridas no está en silencio")
	}
}

func TestSilencioToleraMediaCadencia(t *testing.T) {
	// Un cron diario atrasado dos horas no es noticia; uno mudo hace 36 h sí.
	if pipeConUltima(24, 30*time.Hour).EnSilencio(ahora) {
		t.Error("30 h con cadencia de 24 h está dentro del margen (24 × 1,5 = 36)")
	}
	if !pipeConUltima(24, 40*time.Hour).EnSilencio(ahora) {
		t.Error("40 h con cadencia de 24 h debería ser silencio")
	}
}

// Una corrida `ok` de hace tres semanas no es una buena noticia: el silencio tapa al estado.
func TestSilencioTapaElEstado(t *testing.T) {
	p := pipeConUltima(24, 100*time.Hour)
	if p.EstadoActual(ahora) != EstadoEnSilencio {
		t.Errorf("se esperaba en_silencio y quedó %q", p.EstadoActual(ahora))
	}
	if pipeConUltima(24, time.Hour).EstadoActual(ahora) != EstadoOK {
		t.Error("al día debería mostrar el estado de la última corrida")
	}
}
