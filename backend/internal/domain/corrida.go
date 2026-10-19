// Package domain son las entidades y los puertos. No importa nada de infraestructura:
// ni HTTP, ni S3, ni el sistema de archivos. Se prueba sin levantar nada.
package domain

import (
	"encoding/json"
	"sort"
	"time"
)

// Estado de una corrida. Los tres primeros los declara el emisor; EnSilencio no lo declara
// nadie: lo deduce el vigía por ausencia, y es el único que no viene en ningún sobre.
type Estado string

const (
	EstadoOK         Estado = "ok"
	EstadoSinNovedad Estado = "sin_novedad"
	EstadoError      Estado = "error"
	EstadoEnSilencio Estado = "en_silencio"
)

// EsFallo distingue lo que hay que mirar de lo que no. sin_novedad NO es un fallo: los ETL
// salen con código 0 cuando la fuente no republicó, y pintarlo de rojo hace que en dos
// semanas dejes de mirar el panel.
func (e Estado) EsFallo() bool { return e == EstadoError || e == EstadoEnSilencio }

func (e Estado) Valido() bool {
	switch e {
	case EstadoOK, EstadoSinNovedad, EstadoError:
		return true
	}
	return false
}

// Evento es una línea del .jsonl del emisor. Los campos conocidos se extraen; el resto se
// preserva tal cual en Campos, porque cada pipeline emite los suyos y el consumer no puede
// tener un esquema cerrado sin romperse con el próximo emisor.
type Evento struct {
	TS     time.Time      `json:"ts"`
	Nivel  string         `json:"lvl"`
	Ev     string         `json:"ev"`
	Etapa  string         `json:"etapa,omitempty"`
	Campos map[string]any `json:"campos,omitempty"`
}

// Peso ordena los niveles para poder filtrar "de warn para arriba".
func PesoNivel(n string) int {
	switch n {
	case "debug":
		return 10
	case "info":
		return 20
	case "warn":
		return 30
	case "error":
		return 40
	}
	return 20
}

type ArchivoContexto struct {
	Archivo   string `json:"archivo"`
	Bytes     int    `json:"bytes"`
	Contenido string `json:"contenido,omitempty"`
}

// Corrida es un sobre ya normalizado.
type Corrida struct {
	Proyecto string `json:"proyecto"`
	Pipeline string `json:"pipeline"`
	RunID    string `json:"run_id"`

	Entorno string `json:"entorno,omitempty"`
	Version string `json:"version,omitempty"`
	Host    string `json:"host,omitempty"`
	Comando string `json:"comando,omitempty"`

	Estado     Estado         `json:"status"`
	Inicio     time.Time      `json:"inicio"`
	Segundos   float64        `json:"segundos"`
	Contadores map[string]any `json:"contadores,omitempty"`

	Eventos  []Evento          `json:"eventos,omitempty"`
	Contexto []ArchivoContexto `json:"contexto,omitempty"`

	// Metadatos que pone el consumer, no el emisor.
	Recibido time.Time `json:"recibido"`
	Bytes    int       `json:"bytes"`
}

func (c Corrida) Ref() Ref { return Ref{Proyecto: c.Proyecto, Pipeline: c.Pipeline, RunID: c.RunID} }

// Ref localiza una corrida. El layout de claves está anidado por proyecto/pipeline, así que
// con el run_id solo hay que buscarla; con la Ref completa se va derecho.
type Ref struct {
	Proyecto string `json:"proyecto"`
	Pipeline string `json:"pipeline"`
	RunID    string `json:"run_id"`
}

func (r Ref) Completa() bool { return r.Proyecto != "" && r.Pipeline != "" && r.RunID != "" }

// Etapa es lo que la vista de corrida dibuja: se reconstruye a partir de los eventos
// stage.start / stage.end / stage.error, que es como los emiten todos los pipelines.
type Etapa struct {
	Nombre  string   `json:"nombre"`
	MS      float64  `json:"ms"`
	Estado  string   `json:"estado"` // ok | error | abierta
	Error   string   `json:"error,omitempty"`
	Eventos []Evento `json:"eventos"`
	Salida  []string `json:"salida"` // la salida cruda del subproceso, línea por línea
}

// Etapas reconstruye la ejecución. Los eventos que no caen dentro de ninguna etapa —run.start,
// run.end y los que emite el pipeline entre etapas— van a una etapa sintética "(corrida)",
// para que la vista no pierda nada.
func (c Corrida) Etapas() []Etapa {
	const fuera = "(corrida)"
	orden := []string{}
	por := map[string]*Etapa{}

	obtener := func(nombre string) *Etapa {
		if e, ok := por[nombre]; ok {
			return e
		}
		// Listas vacías, no nil: el JSON de una etapa sin eventos debe decir [] y no null,
		// para que el front no tenga que defenderse de dos formas de "no hay nada".
		e := &Etapa{Nombre: nombre, Estado: "abierta", Eventos: []Evento{}, Salida: []string{}}
		por[nombre] = e
		orden = append(orden, nombre)
		return e
	}

	actual := fuera
	for _, ev := range c.Eventos {
		nombre := ev.Etapa
		switch ev.Ev {
		case "stage.start":
			if nombre != "" {
				actual = nombre
			}
			obtener(actual)
			continue
		case "stage.end", "stage.error":
			if nombre == "" {
				nombre = actual
			}
			e := obtener(nombre)
			e.MS = flotante(ev.Campos["ms"])
			if ev.Ev == "stage.error" {
				e.Estado = "error"
				e.Error = texto(ev.Campos["error"])
			} else {
				e.Estado = "ok"
			}
			actual = fuera
			continue
		}

		destino := actual
		if nombre != "" {
			destino = nombre
		}
		e := obtener(destino)
		// etapa.salida trae la salida cruda del subproceso en el campo `linea`. Va a Salida y
		// NO a Eventos: en una corrida de etl_mensual son 617 líneas, y mandarlas dos veces
		// duplicaría el peso de la respuesta sin agregar nada.
		if ev.Ev == "etapa.salida" {
			if l := texto(ev.Campos["linea"]); l != "" {
				e.Salida = append(e.Salida, l)
				continue
			}
		}
		e.Eventos = append(e.Eventos, ev)
	}

	out := make([]Etapa, 0, len(orden))
	for _, n := range orden {
		e := *por[n]
		// La etapa sintética no es una etapa: no tiene estado propio y pintarla de "abierta"
		// haría parecer que algo quedó colgado.
		if e.Nombre == fuera {
			e.Estado = ""
		}
		out = append(out, e)
	}
	return out
}

// ResumenCorrida es lo que se guarda aparte para poder listar sin abrir el sobre entero.
// Ronda 1 KB; el sobre completo puede ser 53 KB o más.
type ResumenCorrida struct {
	Proyecto string `json:"proyecto"`
	Pipeline string `json:"pipeline"`
	RunID    string `json:"run_id"`

	Entorno string `json:"entorno,omitempty"`
	Version string `json:"version,omitempty"`
	Host    string `json:"host,omitempty"`
	Comando string `json:"comando,omitempty"`

	Estado     Estado         `json:"status"`
	Inicio     time.Time      `json:"inicio"`
	Segundos   float64        `json:"segundos"`
	Contadores map[string]any `json:"contadores,omitempty"`

	NumEventos int       `json:"num_eventos"`
	NumErrores int       `json:"num_errores"`
	NumEtapas  int       `json:"num_etapas"`
	Error      string    `json:"error,omitempty"`       // el primer stage.error, si hubo
	EtapaFallo string    `json:"etapa_fallo,omitempty"` // dónde murió
	TieneCtx   bool      `json:"tiene_contexto"`
	BytesSobre int       `json:"bytes_sobre"`
	Recibido   time.Time `json:"recibido"`
}

func (c Corrida) Resumen() ResumenCorrida {
	r := ResumenCorrida{
		Proyecto: c.Proyecto, Pipeline: c.Pipeline, RunID: c.RunID,
		Entorno: c.Entorno, Version: c.Version, Host: c.Host, Comando: c.Comando,
		Estado: c.Estado, Inicio: c.Inicio, Segundos: c.Segundos, Contadores: c.Contadores,
		NumEventos: len(c.Eventos), BytesSobre: c.Bytes, Recibido: c.Recibido,
	}
	etapas := c.Etapas()
	for _, e := range etapas {
		if e.Nombre != "(corrida)" {
			r.NumEtapas++
		}
		if e.Estado == "error" && r.Error == "" {
			r.Error, r.EtapaFallo = e.Error, e.Nombre
		}
	}
	for _, ev := range c.Eventos {
		if ev.Nivel == "error" {
			r.NumErrores++
		}
		if r.Error == "" && ev.Ev == "run.error" {
			r.Error = texto(ev.Campos["error"])
		}
	}
	for _, a := range c.Contexto {
		if a.Contenido != "" {
			r.TieneCtx = true
			break
		}
	}
	return r
}

// Novedad devuelve el evento de dominio declarado para el pipeline: de los ~9 eventos que
// emite una corrida, ocho dicen si el pipeline funcionó y uno dice si pasó algo en el mundo.
// Ese es el que va en la portada.
func (c Corrida) Novedad(nombreEvento string) (Evento, bool) {
	if nombreEvento == "" {
		return Evento{}, false
	}
	for i := len(c.Eventos) - 1; i >= 0; i-- {
		if c.Eventos[i].Ev == nombreEvento {
			return c.Eventos[i], true
		}
	}
	return Evento{}, false
}

// OrdenarPorInicio deja lo más reciente primero, que es como se mira un panel.
func OrdenarPorInicio(rs []ResumenCorrida) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Inicio.After(rs[j].Inicio) })
}

func texto(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func flotante(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return 0
}
