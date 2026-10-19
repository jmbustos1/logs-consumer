// Package lclog es el cliente de logs-consumer para pipelines en Go.
//
// Copiá este archivo al pipeline y ajustá el nombre del paquete. No tiene dependencias fuera
// de la biblioteca estándar, a propósito: un cliente de telemetría que arrastra dependencias
// es un cliente que nadie adopta.
//
//	r := lclog.Nueva("mi-proyecto", "mi_pipeline")
//	defer r.Cerrar("ok")
//
//	if err := r.Etapa("descargar", func() error { return descargar(ctx) }); err != nil {
//	    r.Cerrar("error")
//	    return err
//	}
//	r.Contador("filas", filas)
//
// Las tres reglas del emisor están implementadas acá y no son configurables:
//  1. Enviar() nunca devuelve un error que rompa la corrida;
//  2. un solo POST al final;
//  3. el .jsonl local se escribe siempre, aunque el envío falle.
package lclog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Evento struct {
	TS    string `json:"ts"`
	Nivel string `json:"lvl"`
	Ev    string `json:"ev"`
	Etapa string `json:"etapa,omitempty"`
	// Los campos libres se aplanan en el JSON: cada pipeline manda los suyos.
	Campos map[string]any `json:"-"`
}

func (e Evento) MarshalJSON() ([]byte, error) {
	m := map[string]any{"ts": e.TS, "lvl": e.Nivel, "ev": e.Ev}
	if e.Etapa != "" {
		m["etapa"] = e.Etapa
	}
	for k, v := range e.Campos {
		m[k] = v
	}
	return json.Marshal(m)
}

type Archivo struct {
	Archivo   string `json:"archivo"`
	Bytes     int    `json:"bytes"`
	Contenido string `json:"contenido,omitempty"`
}

type Run struct {
	mu         sync.Mutex
	RunID      string
	proyecto   string
	pipeline   string
	inicio     time.Time
	eventos    []Evento
	contadores map[string]any
	contexto   []string // rutas; el contenido se lee solo si la corrida falla
	archivo    *os.File
}

func Nueva(proyecto, pipeline string) *Run {
	r := &Run{
		RunID:      nuevoRunID(),
		proyecto:   proyecto,
		pipeline:   pipeline,
		inicio:     time.Now().UTC(),
		contadores: map[string]any{},
	}
	dir := env("LC_LOGS", "./logs")
	if os.MkdirAll(dir, 0o755) == nil {
		r.archivo, _ = os.Create(filepath.Join(dir, r.RunID+".jsonl"))
	}
	host, _ := os.Hostname()
	r.Evento("run.start", "info", "", map[string]any{
		"proyecto": proyecto, "pipeline": pipeline, "host": host,
		"entorno": env("LC_ENTORNO", "local"), "version": env("LC_VERSION", ""),
	})
	return r
}

// runID en formato AAAAMMDD-HHMMSS-hex: el consumer se apoya en que ordena cronológicamente.
func nuevoRunID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%03x", time.Now().UTC().Format("20060102-150405"), int(b[0])<<4|int(b[1])>>4)
}

func (r *Run) Evento(ev, nivel, etapa string, campos map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := Evento{TS: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"), Nivel: nivel, Ev: ev, Etapa: etapa, Campos: campos}
	r.eventos = append(r.eventos, e)
	if r.archivo != nil {
		if b, err := json.Marshal(e); err == nil {
			_, _ = r.archivo.Write(append(b, '\n'))
		}
	}
}

// Salida registra una línea de salida cruda de un subproceso. El panel la muestra en un
// bloque de consola, y es lo que hace que la vista sea un log y no un resumen.
func (r *Run) Salida(etapa, linea string) {
	r.Evento("etapa.salida", "debug", etapa, map[string]any{"linea": linea})
}

func (r *Run) Contador(nombre string, valor any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contadores[nombre] = valor
}

// Contexto declara qué archivos adjuntar SI la corrida falla: el código del pipeline y los
// documentos que explican lo que el código no dice. Nunca secretos.
func (r *Run) Contexto(rutas ...string) { r.contexto = append(r.contexto, rutas...) }

// Etapa envuelve un paso. Emite stage.start y stage.end/stage.error con su duración.
func (r *Run) Etapa(nombre string, fn func() error) error {
	t0 := time.Now()
	r.Evento("stage.start", "info", nombre, nil)
	err := fn()
	ms := float64(time.Since(t0).Microseconds()) / 1000
	if err != nil {
		r.Evento("stage.error", "error", nombre, map[string]any{"ms": ms, "error": err.Error()})
		return err
	}
	r.Evento("stage.end", "info", nombre, map[string]any{"ms": ms})
	return nil
}

// Cerrar escribe run.end y despacha. Nunca devuelve error: si el consumer no responde, la
// corrida terminó igual y el .jsonl local ya está en disco.
func (r *Run) Cerrar(status string) {
	segundos := time.Since(r.inicio).Seconds()
	r.mu.Lock()
	campos := map[string]any{"status": status, "segundos": redondear(segundos)}
	for k, v := range r.contadores {
		campos[k] = v
	}
	r.mu.Unlock()
	r.Evento("run.end", "info", "", campos)
	if r.archivo != nil {
		_ = r.archivo.Close()
	}
	r.enviar(status, segundos)
}

func (r *Run) enviar(status string, segundos float64) {
	url := env("LC_URL", "")
	if url == "" {
		return // sin destino no se envía nada, y eso no es un error
	}
	host, _ := os.Hostname()
	sobre := map[string]any{
		"proyecto": r.proyecto, "pipeline": r.pipeline, "run_id": r.RunID,
		"status": status, "inicio": r.inicio.Format(time.RFC3339),
		"segundos": redondear(segundos), "host": host,
		"entorno": env("LC_ENTORNO", "local"), "version": env("LC_VERSION", ""),
		"contadores": r.contadores, "eventos": r.eventos,
	}
	// El contenido del contexto viaja SOLO cuando la corrida falla.
	if status == "error" {
		var archivos []Archivo
		for _, ruta := range r.contexto {
			datos, err := os.ReadFile(ruta)
			if err != nil {
				continue
			}
			archivos = append(archivos, Archivo{Archivo: filepath.Base(ruta), Bytes: len(datos), Contenido: string(datos)})
		}
		sobre["contexto"] = archivos
	}

	cuerpo, err := json.Marshal(sobre)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] envio.error no se pudo serializar: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if t := env("LC_TOKEN", ""); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Deliberadamente amplio. DNS, TLS, timeout, el consumer a medio construir: nada de
		// eso es relevante para el resultado del pipeline.
		fmt.Fprintf(os.Stderr, "  [warn] envio.error %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "  [warn] envio.error codigo=%d\n", resp.StatusCode)
		return
	}
	fmt.Fprintf(os.Stderr, "  [info] envio.ok codigo=%d bytes=%d\n", resp.StatusCode, len(cuerpo))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func redondear(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
