package domain

import (
	"context"
	"time"
)

// Almacen son las tres primitivas sobre objetos. `disco` y `s3` lo implementan y no saben
// nada del dominio: reciben una clave y bytes.
type Almacen interface {
	Put(ctx context.Context, clave string, datos []byte) error
	Get(ctx context.Context, clave string) ([]byte, error)
	List(ctx context.Context, prefijo string) ([]Objeto, error)
	Delete(ctx context.Context, clave string) error
}

type Objeto struct {
	Clave      string
	Bytes      int64
	Modificado time.Time
}

// Repositorio es lo que necesitan los casos de uso. Tiene UNA implementación, montada sobre
// Almacen: el layout de claves se decide una vez y vale igual en disco y en S3.
type Repositorio interface {
	GuardarCorrida(ctx context.Context, c Corrida, bruto []byte) error
	LeerCorrida(ctx context.Context, ref Ref) (Corrida, error)
	LeerSobre(ctx context.Context, ref Ref) ([]byte, error)
	BuscarRun(ctx context.Context, runID string) (Ref, error)
	ListarCorridas(ctx context.Context, f Filtro) ([]ResumenCorrida, error)

	GuardarPipeline(ctx context.Context, p Pipeline) error
	EliminarPipeline(ctx context.Context, proyecto, pipeline string) (Borrado, error)
	LeerPipeline(ctx context.Context, proyecto, pipeline string) (Pipeline, error)
	ListarPipelines(ctx context.Context) ([]Pipeline, error)
}

// Borrado es el recuento de lo que se fue. Se devuelve y se registra: una operación
// destructiva que no dice cuánto destruyó es una que nadie puede auditar después.
type Borrado struct {
	Sobres    int `json:"sobres"`
	Resumenes int `json:"resumenes"`
	Estado    int `json:"estado"`
}

func (b Borrado) Total() int { return b.Sobres + b.Resumenes + b.Estado }

type Filtro struct {
	Proyecto string
	Pipeline string
	Estado   Estado
	Desde    string // run_id o fecha AAAAMMDD; se compara como prefijo, que para run_id es cronológico
	Hasta    string
	Limite   int
}

// Notificador es el enchufe de alertas. Hoy no hay canal elegido: la implementación por
// defecto no hace nada y lo dice. Mismas reglas que el envio.py de los emisores — nunca
// puede tumbar lo que está anotando.
type Notificador interface {
	Avisar(ctx context.Context, asunto, cuerpo string) error
}

// Diagnosticador convierte el bundle en una propuesta de arreglo. Viene apagado.
type Diagnosticador interface {
	Activo() bool
	Proponer(ctx context.Context, bundle string) (Propuesta, error)
}

type Propuesta struct {
	Modelo    string  `json:"modelo"`
	Texto     string  `json:"texto"`
	TokensEnt int     `json:"tokens_entrada"`
	TokensSal int     `json:"tokens_salida"`
	CostoUSD  float64 `json:"costo_usd"`
	Segundos  float64 `json:"segundos"`
}
