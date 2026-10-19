package almacen

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"logs-consumer/internal/domain"
)

// Repo implementa domain.Repositorio sobre cualquier Almacen. Es la única implementación:
// cambiar de disco a S3 cambia el Almacen, no esto.
type Repo struct {
	a domain.Almacen
}

func NuevoRepo(a domain.Almacen) *Repo { return &Repo{a: a} }

const limiteDefecto = 50

func (r *Repo) GuardarCorrida(ctx context.Context, c domain.Corrida, bruto []byte) error {
	ref := c.Ref()

	// El sobre crudo, comprimido, es la fuente de verdad: todo lo demás se puede reconstruir
	// desde acá. Se guarda primero, por eso mismo.
	comprimido, err := comprimir(bruto)
	if err != nil {
		return fmt.Errorf("comprimiendo el sobre: %w", err)
	}
	if err := r.a.Put(ctx, ClaveSobre(ref), comprimido); err != nil {
		return fmt.Errorf("guardando el sobre: %w", err)
	}

	// Reenviar el mismo run_id con otro status dejaría dos resúmenes y la corrida aparecería
	// dos veces en el listado. Se borran los previos.
	previos, _ := r.a.List(ctx, PrefijoResumenRun(ref))
	nueva := ClaveResumen(ref, c.Estado)
	for _, o := range previos {
		if o.Clave != nueva {
			_ = r.a.Delete(ctx, o.Clave)
		}
	}

	res := c.Resumen()
	datos, err := json.Marshal(res)
	if err != nil {
		return err
	}
	if err := r.a.Put(ctx, nueva, datos); err != nil {
		return fmt.Errorf("guardando el resumen: %w", err)
	}

	return r.actualizarPipeline(ctx, res, len(previos) == 0)
}

// actualizarPipeline mantiene el objeto que lee la portada. Es un read-modify-write, pero un
// pipeline nunca corre concurrentemente consigo mismo; y si se corrompiera, se reconstruye
// releyendo sobres/.
func (r *Repo) actualizarPipeline(ctx context.Context, res domain.ResumenCorrida, esNueva bool) error {
	p, err := r.LeerPipeline(ctx, res.Proyecto, res.Pipeline)
	if err != nil {
		p = domain.Pipeline{Proyecto: res.Proyecto, Pipeline: res.Pipeline}
	}
	if esNueva {
		p.Corridas++
	}
	// Una corrida vieja reenviada no debe pisar a la última.
	if p.Ultima == nil || !res.Inicio.Before(p.Ultima.Inicio) {
		copia := res
		p.Ultima = &copia
	}
	p.Actualizado = time.Now().UTC()
	return r.GuardarPipeline(ctx, p)
}

func (r *Repo) LeerSobre(ctx context.Context, ref domain.Ref) ([]byte, error) {
	datos, err := r.a.Get(ctx, ClaveSobre(ref))
	if err != nil {
		return nil, err
	}
	return descomprimir(datos)
}

func (r *Repo) LeerCorrida(ctx context.Context, ref domain.Ref) (domain.Corrida, error) {
	bruto, err := r.LeerSobre(ctx, ref)
	if err != nil {
		return domain.Corrida{}, err
	}
	c, _, err := domain.DesdeSobre(bruto, time.Now().UTC())
	if err != nil {
		return domain.Corrida{}, err
	}
	// El sobre guardado ya pasó por la normalización una vez; los metadatos que puso el
	// consumer viven en el resumen, así que se recuperan de ahí.
	if res, err := r.leerResumenDe(ctx, ref); err == nil {
		c.Recibido = res.Recibido
		c.Bytes = res.BytesSobre
	}
	return c, nil
}

func (r *Repo) leerResumenDe(ctx context.Context, ref domain.Ref) (domain.ResumenCorrida, error) {
	objs, err := r.a.List(ctx, PrefijoResumenRun(ref))
	if err != nil || len(objs) == 0 {
		return domain.ResumenCorrida{}, domain.ErrNoEncontrado
	}
	datos, err := r.a.Get(ctx, objs[0].Clave)
	if err != nil {
		return domain.ResumenCorrida{}, err
	}
	var res domain.ResumenCorrida
	return res, json.Unmarshal(datos, &res)
}

// BuscarRun resuelve un run_id suelto. El layout está anidado por proyecto/pipeline, así que
// hay que listar; a este volumen es un List y un filtro en memoria.
func (r *Repo) BuscarRun(ctx context.Context, runID string) (domain.Ref, error) {
	objs, err := r.a.List(ctx, PrefResumen)
	if err != nil {
		return domain.Ref{}, err
	}
	for _, o := range objs {
		if ref, _, ok := LeerClaveResumen(o.Clave); ok && ref.RunID == runID {
			return ref, nil
		}
	}
	return domain.Ref{}, fmt.Errorf("%w: corrida %s", domain.ErrNoEncontrado, runID)
}

func (r *Repo) ListarCorridas(ctx context.Context, f domain.Filtro) ([]domain.ResumenCorrida, error) {
	objs, err := r.a.List(ctx, PrefijoResumen(f.Proyecto, f.Pipeline))
	if err != nil {
		return nil, err
	}

	// Filtrar por CLAVE, antes de abrir nada: el estado y la fecha están en el nombre.
	type cand struct {
		clave string
		ref   domain.Ref
	}
	var cands []cand
	for _, o := range objs {
		ref, estado, ok := LeerClaveResumen(o.Clave)
		if !ok {
			continue
		}
		if f.Pipeline != "" && ref.Pipeline != f.Pipeline {
			continue
		}
		if f.Estado != "" && estado != f.Estado {
			continue
		}
		if f.Desde != "" && ref.RunID < f.Desde {
			continue
		}
		if f.Hasta != "" && ref.RunID > f.Hasta {
			continue
		}
		cands = append(cands, cand{o.Clave, ref})
	}

	// run_id ordena cronológicamente: lo más reciente primero.
	sort.Slice(cands, func(i, j int) bool { return cands[i].ref.RunID > cands[j].ref.RunID })
	limite := f.Limite
	if limite <= 0 {
		limite = limiteDefecto
	}
	if len(cands) > limite {
		cands = cands[:limite]
	}

	out := make([]domain.ResumenCorrida, len(cands))
	var wg sync.WaitGroup
	for i, c := range cands {
		wg.Add(1)
		go func(i int, clave string) {
			defer wg.Done()
			datos, err := r.a.Get(ctx, clave)
			if err != nil {
				return
			}
			var res domain.ResumenCorrida
			if json.Unmarshal(datos, &res) == nil {
				out[i] = res
			}
		}(i, c.clave)
	}
	wg.Wait()

	limpio := out[:0]
	for _, res := range out {
		if res.RunID != "" {
			limpio = append(limpio, res)
		}
	}
	return limpio, nil
}

func (r *Repo) GuardarPipeline(ctx context.Context, p domain.Pipeline) error {
	datos, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.a.Put(ctx, ClavePipeline(p.Proyecto, p.Pipeline), datos)
}

func (r *Repo) LeerPipeline(ctx context.Context, proyecto, pipeline string) (domain.Pipeline, error) {
	datos, err := r.a.Get(ctx, ClavePipeline(proyecto, pipeline))
	if err != nil {
		return domain.Pipeline{}, err
	}
	var p domain.Pipeline
	if err := json.Unmarshal(datos, &p); err != nil {
		return domain.Pipeline{}, err
	}
	return p, nil
}

// EliminarPipeline borra TODO lo de un pipeline: su estado, sus resúmenes y sus sobres.
//
// Existe por un caso concreto y repetible: un emisor mal configurado —un guion donde iba un
// guion bajo— crea un pipeline fantasma que el panel muestra para siempre. Sin esto, la única
// salida es borrar objetos a mano en S3, y eso no debería ser parte de la operación normal.
//
// Borra también los sobres, que son la fuente de verdad. Es deliberado: un fantasma del que
// quedan los crudos vuelve a aparecer en cuanto alguien reconstruya el índice.
func (r *Repo) EliminarPipeline(ctx context.Context, proyecto, pipeline string) (domain.Borrado, error) {
	var b domain.Borrado
	ref := domain.Ref{Proyecto: proyecto, Pipeline: pipeline}

	for _, par := range []struct {
		prefijo string
		cuenta  *int
	}{
		{PrefijoResumen(proyecto, pipeline), &b.Resumenes},
		{fmt.Sprintf("%s%s/%s/", PrefSobres, proyecto, pipeline), &b.Sobres},
	} {
		objs, err := r.a.List(ctx, par.prefijo)
		if err != nil {
			return b, err
		}
		for _, o := range objs {
			if err := r.a.Delete(ctx, o.Clave); err != nil {
				return b, fmt.Errorf("borrando %s: %w", o.Clave, err)
			}
			*par.cuenta++
		}
	}

	// El objeto de estado va último: si algo falla antes, el pipeline sigue siendo visible en
	// el panel y se nota que quedó a medias.
	if _, err := r.a.Get(ctx, ClavePipeline(proyecto, pipeline)); err == nil {
		if err := r.a.Delete(ctx, ClavePipeline(proyecto, pipeline)); err != nil {
			return b, err
		}
		b.Estado = 1
	}
	_ = ref
	return b, nil
}

func (r *Repo) ListarPipelines(ctx context.Context) ([]domain.Pipeline, error) {
	objs, err := r.a.List(ctx, PrefPipelines)
	if err != nil {
		return nil, err
	}
	var out []domain.Pipeline
	for _, o := range objs {
		if !strings.HasSuffix(o.Clave, ".json") {
			continue
		}
		datos, err := r.a.Get(ctx, o.Clave)
		if err != nil {
			continue
		}
		var p domain.Pipeline
		if json.Unmarshal(datos, &p) == nil && p.Pipeline != "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Proyecto != out[j].Proyecto {
			return out[i].Proyecto < out[j].Proyecto
		}
		return out[i].Pipeline < out[j].Pipeline
	})
	return out, nil
}

func comprimir(datos []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(datos); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func descomprimir(datos []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(datos))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
