package almacen

import (
	"context"
	"testing"
	"time"

	"logs-consumer/internal/adapter/almacen/disco"
	"logs-consumer/internal/domain"
)

func repoDePrueba(t *testing.T) *Repo {
	t.Helper()
	a, err := disco.Nuevo(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NuevoRepo(a)
}

func sobre(runID, estado string) ([]byte, domain.Corrida) {
	bruto := []byte(`{"proyecto":"p","pipeline":"q","run_id":"` + runID + `","status":"` + estado + `",
	  "segundos":1.5,"inicio":"2026-08-31T06:00:00Z","contadores":{"filas":10},
	  "eventos":[{"ts":"2026-08-31T06:00:00Z","lvl":"info","ev":"run.start"}]}`)
	c, _, err := domain.DesdeSobre(bruto, time.Now().UTC())
	if err != nil {
		panic(err)
	}
	return bruto, c
}

func TestIdaYVuelta(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	bruto, c := sobre("20260831-060000-aaa", "ok")

	if err := r.GuardarCorrida(ctx, c, bruto); err != nil {
		t.Fatal(err)
	}
	leida, err := r.LeerCorrida(ctx, c.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if leida.RunID != c.RunID || leida.Estado != domain.EstadoOK || leida.Segundos != 1.5 {
		t.Errorf("la corrida no volvió igual: %+v", leida)
	}

	// El sobre crudo es la fuente de verdad: tiene que volver byte por byte.
	crudo, err := r.LeerSobre(ctx, c.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if string(crudo) != string(bruto) {
		t.Error("el sobre crudo no volvió idéntico")
	}
}

// Reenviar la misma corrida no puede duplicarla ni inflar el contador del pipeline; y si vuelve
// con otro estado, el resumen viejo tiene que desaparecer o el listado la mostraría dos veces.
func TestIdempotenciaYCambioDeEstado(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()

	bruto, c := sobre("20260831-060000-aaa", "ok")
	for i := 0; i < 3; i++ {
		if err := r.GuardarCorrida(ctx, c, bruto); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := r.ListarCorridas(ctx, domain.Filtro{})
	if len(rs) != 1 {
		t.Fatalf("tres envíos de la misma corrida dejaron %d entradas", len(rs))
	}
	p, _ := r.LeerPipeline(ctx, "p", "q")
	if p.Corridas != 1 {
		t.Errorf("el contador del pipeline se infló: %d", p.Corridas)
	}

	bruto2, c2 := sobre("20260831-060000-aaa", "error")
	if err := r.GuardarCorrida(ctx, c2, bruto2); err != nil {
		t.Fatal(err)
	}
	rs, _ = r.ListarCorridas(ctx, domain.Filtro{})
	if len(rs) != 1 || rs[0].Estado != domain.EstadoError {
		t.Errorf("tras cambiar de estado quedaron %d entradas: %+v", len(rs), rs)
	}
}

// El estado va en el nombre del objeto: filtrar por estado no debe abrir un solo objeto.
func TestFiltroPorEstadoYOrden(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	for _, par := range [][2]string{
		{"20260831-060000-aaa", "ok"},
		{"20260831-070000-bbb", "error"},
		{"20260831-080000-ccc", "sin_novedad"},
	} {
		b, c := sobre(par[0], par[1])
		if err := r.GuardarCorrida(ctx, c, b); err != nil {
			t.Fatal(err)
		}
	}

	solo, _ := r.ListarCorridas(ctx, domain.Filtro{Estado: domain.EstadoError})
	if len(solo) != 1 || solo[0].RunID != "20260831-070000-bbb" {
		t.Errorf("el filtro por estado falló: %+v", solo)
	}

	// run_id es cronológico, así que listar es ordenar claves: lo más reciente primero.
	todas, _ := r.ListarCorridas(ctx, domain.Filtro{})
	if len(todas) != 3 || todas[0].RunID != "20260831-080000-ccc" {
		t.Errorf("el orden no es del más reciente al más viejo: %+v", todas)
	}

	// La última corrida del pipeline es la más nueva, no la última guardada.
	p, _ := r.LeerPipeline(ctx, "p", "q")
	if p.Ultima == nil || p.Ultima.RunID != "20260831-080000-ccc" {
		t.Errorf("la última corrida del pipeline es la equivocada: %+v", p.Ultima)
	}
}

func TestBuscarRunSinSaberElPipeline(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	b, c := sobre("20260831-060000-aaa", "ok")
	_ = r.GuardarCorrida(ctx, c, b)

	ref, err := r.BuscarRun(ctx, "20260831-060000-aaa")
	if err != nil || ref.Proyecto != "p" || ref.Pipeline != "q" {
		t.Errorf("no resolvió el run suelto: %+v %v", ref, err)
	}
	if _, err := r.BuscarRun(ctx, "no-existe"); err == nil {
		t.Error("buscar una corrida inexistente debería fallar")
	}
}

func TestEliminarPipelineBorraTodo(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()

	b1, c1 := sobre("20260901-060000-aaa", "ok")
	_ = r.GuardarCorrida(ctx, c1, b1)
	b2, c2 := sobre("20260901-070000-bbb", "error")
	_ = r.GuardarCorrida(ctx, c2, b2)

	b, err := r.EliminarPipeline(ctx, "p", "q")
	if err != nil {
		t.Fatal(err)
	}
	if b.Sobres != 2 || b.Resumenes != 2 || b.Estado != 1 {
		t.Errorf("recuento equivocado: %+v", b)
	}

	// Los sobres se van también: un fantasma del que quedan los crudos reaparece en cuanto
	// alguien reconstruya el índice.
	if rs, _ := r.ListarCorridas(ctx, domain.Filtro{}); len(rs) != 0 {
		t.Errorf("quedaron corridas: %+v", rs)
	}
	if ps, _ := r.ListarPipelines(ctx); len(ps) != 0 {
		t.Errorf("quedó el pipeline: %+v", ps)
	}
	if _, err := r.LeerSobre(ctx, c1.Ref()); err == nil {
		t.Error("el sobre crudo sobrevivió al borrado")
	}
}

// Borrar un pipeline no puede llevarse por delante a sus vecinos.
func TestEliminarNoTocaOtrosPipelines(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()

	b1, c1 := sobre("20260901-060000-aaa", "ok")
	_ = r.GuardarCorrida(ctx, c1, b1)

	bruto := []byte(`{"proyecto":"p","pipeline":"otro","run_id":"20260901-060000-zzz","status":"ok",
	  "inicio":"2026-09-01T06:00:00Z","eventos":[{"ts":"2026-09-01T06:00:00Z","lvl":"info","ev":"run.start"}]}`)
	c2, _, err := domain.DesdeSobre(bruto, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.GuardarCorrida(ctx, c2, bruto); err != nil {
		t.Fatal(err)
	}

	if _, err := r.EliminarPipeline(ctx, "p", "q"); err != nil {
		t.Fatal(err)
	}
	ps, _ := r.ListarPipelines(ctx)
	if len(ps) != 1 || ps[0].Pipeline != "otro" {
		t.Errorf("el vecino no sobrevivió intacto: %+v", ps)
	}
	if rs, _ := r.ListarCorridas(ctx, domain.Filtro{}); len(rs) != 1 {
		t.Errorf("se borraron corridas de más: %+v", rs)
	}
}
