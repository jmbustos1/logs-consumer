package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DesdeSobre normaliza el sobre que manda un emisor. Es la única puerta de entrada del
// contrato y es deliberadamente tolerante: exige el mínimo indispensable (proyecto, pipeline,
// run_id) y preserva todo lo que no conoce.
//
// La tolerancia no es pereza: los tres ETL de leads-searcher emiten `etapa`, lids-searcher
// emite `stage`, y el próximo pipeline va a emitir otra cosa. Si conectar un emisor nuevo
// exigiera negociar un esquema, nadie conectaría el segundo.
//
// Devuelve avisos —cosas raras pero tolerables— que POST /corridas/validar le muestra a quien
// está integrando. Un aviso no impide guardar; un error sí.
func DesdeSobre(bruto []byte, recibido time.Time) (Corrida, []string, error) {
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(bruto)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return Corrida{}, nil, fmt.Errorf("%w: el cuerpo no es un objeto JSON válido: %v", ErrInvalido, err)
	}

	var avisos []string
	c := Corrida{
		Proyecto: texto(m["proyecto"]),
		Pipeline: texto(m["pipeline"]),
		Entorno:  texto(m["entorno"]),
		Version:  texto(m["version"]),
		Host:     texto(m["host"]),
		Comando:  texto(m["comando"]),
		Recibido: recibido,
		Bytes:    len(bruto),
	}

	// run_id, o `run` para emisores que llaman así a la corrida.
	c.RunID = texto(m["run_id"])
	if c.RunID == "" {
		c.RunID = texto(m["run"])
		if c.RunID != "" {
			avisos = append(avisos, "se usó el campo `run` como run_id; el contrato dice `run_id`")
		}
	}

	// Orden FIJO, no un mapa: el de un mapa es aleatorio en Go, y con un sobre al que le
	// faltan los tres campos el mensaje cambiaría en cada intento. Quien está integrando un
	// pipeline necesita que el mismo error diga siempre lo mismo.
	for _, campo := range []struct{ nombre, valor string }{
		{"proyecto", c.Proyecto}, {"pipeline", c.Pipeline}, {"run_id", c.RunID},
	} {
		if campo.valor == "" {
			return c, avisos, fmt.Errorf("%w: falta el campo obligatorio `%s`", ErrInvalido, campo.nombre)
		}
		if err := seguro(campo.nombre, campo.valor); err != nil {
			return c, avisos, err
		}
	}

	c.Estado, avisos = normalizarEstado(texto(m["status"]), avisos)
	c.Segundos = flotante(m["segundos"])

	if v, ok := m["contadores"].(map[string]any); ok {
		c.Contadores = v
	}

	// Los eventos son el corazón: sin ellos no hay vista de corrida.
	if lista, ok := m["eventos"].([]any); ok {
		c.Eventos = make([]Evento, 0, len(lista))
		for _, item := range lista {
			if obj, ok := item.(map[string]any); ok {
				c.Eventos = append(c.Eventos, eventoDesde(obj))
			}
		}
	}
	if len(c.Eventos) == 0 {
		avisos = append(avisos, "el sobre no trae eventos: la vista de corrida va a quedar vacía")
	}

	if lista, ok := m["contexto"].([]any); ok {
		for _, item := range lista {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			c.Contexto = append(c.Contexto, ArchivoContexto{
				Archivo:   texto(obj["archivo"]),
				Bytes:     int(flotante(obj["bytes"])),
				Contenido: texto(obj["contenido"]),
			})
		}
	}
	if c.Estado == EstadoError && len(c.Contexto) == 0 {
		avisos = append(avisos, "corrida fallida sin `contexto`: el bundle de diagnóstico va a salir sin el código del pipeline")
	}

	c.Inicio = fechaInicio(texto(m["inicio"]), c.Eventos, recibido, &avisos)
	// Un inicio en el futuro no es un capricho: hace que el pipeline nunca pueda entrar en
	// silencio, porque "hace cuánto que no habla" da negativo para siempre. Casi siempre es
	// una zona horaria mal puesta en el emisor.
	if c.Inicio.Sub(recibido) > 5*time.Minute {
		avisos = append(avisos, fmt.Sprintf(
			"`inicio` está %s en el futuro: revisá la zona horaria del emisor, o el vigía nunca va a notar su silencio",
			c.Inicio.Sub(recibido).Round(time.Minute)))
	}
	return c, avisos, nil
}

func eventoDesde(obj map[string]any) Evento {
	e := Evento{
		Nivel: texto(obj["lvl"]),
		Ev:    texto(obj["ev"]),
	}
	if e.Nivel == "" {
		e.Nivel = texto(obj["level"])
	}
	if e.Nivel == "" {
		e.Nivel = "info"
	}
	if e.Ev == "" {
		e.Ev = texto(obj["event"])
	}
	// `etapa` en los ETL de leads-searcher, `stage` en lids-searcher. Son lo mismo.
	e.Etapa = texto(obj["etapa"])
	if e.Etapa == "" {
		e.Etapa = texto(obj["stage"])
	}
	e.TS, _ = fecha(texto(obj["ts"]))

	e.Campos = map[string]any{}
	for k, v := range obj {
		switch k {
		case "ts", "lvl", "level", "ev", "event", "run", "etapa", "stage":
			continue
		}
		e.Campos[k] = v
	}
	return e
}

// normalizarEstado NO inventa. Si llega un valor desconocido se conserva tal cual y se avisa:
// mapearlo a `ok` escondería un fallo, y mapearlo a `error` inventaría uno.
func normalizarEstado(s string, avisos []string) (Estado, []string) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return EstadoOK, append(avisos, "el sobre no trae `status`; se asumió `ok`")
	case "ok", "success", "succeeded":
		return EstadoOK, avisos
	case "sin_novedad", "no_change", "unchanged", "nochange":
		return EstadoSinNovedad, avisos
	case "error", "failed", "failure", "fail":
		return EstadoError, avisos
	default:
		return Estado(s), append(avisos,
			fmt.Sprintf("`status` desconocido: %q. El contrato define ok | sin_novedad | error", s))
	}
}

func fechaInicio(s string, eventos []Evento, recibido time.Time, avisos *[]string) time.Time {
	if t, ok := fecha(s); ok {
		return t
	}
	for _, e := range eventos {
		if !e.TS.IsZero() {
			*avisos = append(*avisos, "`inicio` ausente o ilegible; se usó el ts del primer evento")
			return e.TS
		}
	}
	*avisos = append(*avisos, "`inicio` ausente y sin eventos fechados; se usó la hora de recepción")
	return recibido
}

func fecha(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// seguro impide que un emisor decida dónde se escribe. Estos tres valores forman rutas de
// archivo y claves de S3: un `../` acá sería escritura arbitraria, no un dato feo.
func seguro(campo, v string) error {
	if len(v) > 128 {
		return fmt.Errorf("%w: `%s` supera 128 caracteres", ErrInvalido, campo)
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("%w: `%s` solo admite letras, dígitos, `-`, `_` y `.` (llegó %q)", ErrInvalido, campo, v)
		}
	}
	if strings.Contains(v, "..") {
		return fmt.Errorf("%w: `%s` no puede contener `..`", ErrInvalido, campo)
	}
	return nil
}
