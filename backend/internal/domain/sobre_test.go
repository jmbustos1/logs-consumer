package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var ahora = time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)

func TestExigeLoMinimo(t *testing.T) {
	casos := map[string]string{
		`{}`:                              "proyecto",
		`{"proyecto":"p"}`:                "pipeline",
		`{"proyecto":"p","pipeline":"q"}`: "run_id",
		`no soy json`:                     "objeto JSON",
	}
	for cuerpo, esperado := range casos {
		_, _, err := DesdeSobre([]byte(cuerpo), ahora)
		if err == nil || !strings.Contains(err.Error(), esperado) {
			t.Errorf("con %q se esperaba un error sobre %q y llegó %v", cuerpo, esperado, err)
		}
		if err != nil && !errors.Is(err, ErrInvalido) {
			t.Errorf("con %q el error debería ser ErrInvalido", cuerpo)
		}
	}
}

// Los tres campos que forman rutas de archivo y claves de S3. Un `../` acá sería escritura
// arbitraria, no un dato feo.
func TestRechazaRutas(t *testing.T) {
	for _, malo := range []string{"../etc", "a/b", "con espacio", "..", strings.Repeat("x", 200)} {
		cuerpo := `{"proyecto":"` + malo + `","pipeline":"q","run_id":"r"}`
		if _, _, err := DesdeSobre([]byte(cuerpo), ahora); err == nil {
			t.Errorf("se aceptó un proyecto peligroso: %q", malo)
		}
	}
}

func TestNormalizaEstados(t *testing.T) {
	casos := map[string]Estado{
		`"ok"`:          EstadoOK,
		`"success"`:     EstadoOK,
		`"failed"`:      EstadoError,
		`"no_change"`:   EstadoSinNovedad,
		`"sin_novedad"`: EstadoSinNovedad,
	}
	for entrada, esperado := range casos {
		c, _, err := DesdeSobre([]byte(`{"proyecto":"p","pipeline":"q","run_id":"r","status":`+entrada+`}`), ahora)
		if err != nil {
			t.Fatal(err)
		}
		if c.Estado != esperado {
			t.Errorf("status %s -> %s, se esperaba %s", entrada, c.Estado, esperado)
		}
	}
}

// Un estado desconocido NO se mapea: mandarlo a `ok` escondería un fallo y mandarlo a `error`
// inventaría uno. Se conserva y se avisa.
func TestEstadoDesconocidoSeConserva(t *testing.T) {
	c, avisos, err := DesdeSobre([]byte(`{"proyecto":"p","pipeline":"q","run_id":"r","status":"raro"}`), ahora)
	if err != nil {
		t.Fatal(err)
	}
	if c.Estado != Estado("raro") {
		t.Errorf("se esperaba conservar 'raro' y quedó %q", c.Estado)
	}
	if len(avisos) == 0 {
		t.Error("un status desconocido tiene que dejar aviso")
	}
}

// sin_novedad es el estado que se malinterpreta: significa que la corrida salió bien y no
// había nada que hacer. Tratarlo como fallo llena el panel de rojo y hace que se deje de mirar.
func TestSinNovedadNoEsFallo(t *testing.T) {
	if EstadoSinNovedad.EsFallo() {
		t.Error("sin_novedad NO es un fallo")
	}
	if !EstadoError.EsFallo() || !EstadoEnSilencio.EsFallo() {
		t.Error("error y en_silencio sí son fallos")
	}
}

// Un emisor nuevo va a llamar `stage` a lo que otro llama `etapa`, y va a mandar campos que el
// consumer nunca vio. Ninguna de las dos cosas puede exigir cambiar el consumer.
func TestToleraEmisoresDistintos(t *testing.T) {
	cuerpo := `{"proyecto":"p","pipeline":"q","run_id":"r","status":"ok","eventos":[
	  {"ts":"2026-08-31T06:00:00Z","level":"warn","event":"algo.raro","stage":"descargar","inventado":42}]}`
	c, _, err := DesdeSobre([]byte(cuerpo), ahora)
	if err != nil {
		t.Fatal(err)
	}
	ev := c.Eventos[0]
	if ev.Etapa != "descargar" {
		t.Errorf("`stage` debería leerse como etapa, quedó %q", ev.Etapa)
	}
	if ev.Nivel != "warn" || ev.Ev != "algo.raro" {
		t.Errorf("`level`/`event` no se normalizaron: %+v", ev)
	}
	if ev.Campos["inventado"] == nil {
		t.Error("los campos desconocidos tienen que preservarse")
	}
}

func TestEtapasSeReconstruyen(t *testing.T) {
	cuerpo := `{"proyecto":"p","pipeline":"q","run_id":"r","status":"error","eventos":[
	  {"ts":"2026-08-31T06:00:00Z","lvl":"info","ev":"run.start"},
	  {"ts":"2026-08-31T06:00:01Z","lvl":"info","ev":"stage.start","etapa":"bajar"},
	  {"ts":"2026-08-31T06:00:02Z","lvl":"debug","ev":"etapa.salida","etapa":"bajar","linea":"3 archivos"},
	  {"ts":"2026-08-31T06:00:03Z","lvl":"info","ev":"stage.end","etapa":"bajar","ms":2000},
	  {"ts":"2026-08-31T06:00:04Z","lvl":"info","ev":"stage.start","etapa":"validar"},
	  {"ts":"2026-08-31T06:00:05Z","lvl":"error","ev":"stage.error","etapa":"validar","ms":50,"error":"faltan columnas"}]}`
	c, _, err := DesdeSobre([]byte(cuerpo), ahora)
	if err != nil {
		t.Fatal(err)
	}
	etapas := c.Etapas()
	if len(etapas) != 3 {
		t.Fatalf("se esperaban 3 etapas —la sintética más dos reales— y hay %d", len(etapas))
	}
	bajar := etapas[1]
	if bajar.Nombre != "bajar" || bajar.Estado != "ok" || bajar.MS != 2000 {
		t.Errorf("etapa `bajar` mal reconstruida: %+v", bajar)
	}
	// La salida cruda va a Salida y NO a Eventos: en una corrida real son cientos de líneas
	// y duplicarlas duplicaría el peso de la respuesta.
	if len(bajar.Salida) != 1 || bajar.Salida[0] != "3 archivos" {
		t.Errorf("la salida del subproceso no se separó: %+v", bajar)
	}
	if len(bajar.Eventos) != 0 {
		t.Errorf("etapa.salida no debería quedar también como evento: %+v", bajar.Eventos)
	}
	validar := etapas[2]
	if validar.Estado != "error" || validar.Error != "faltan columnas" {
		t.Errorf("no se detectó dónde murió: %+v", validar)
	}

	res := c.Resumen()
	if res.EtapaFallo != "validar" || res.NumEtapas != 2 {
		t.Errorf("resumen mal: etapa_fallo=%q num_etapas=%d", res.EtapaFallo, res.NumEtapas)
	}
}

func TestInicioSeDeduceDeLosEventos(t *testing.T) {
	cuerpo := `{"proyecto":"p","pipeline":"q","run_id":"r","eventos":[{"ts":"2026-08-31T05:00:00Z","ev":"x"}]}`
	c, avisos, err := DesdeSobre([]byte(cuerpo), ahora)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Inicio.Equal(time.Date(2026, 8, 31, 5, 0, 0, 0, time.UTC)) {
		t.Errorf("inicio mal deducido: %v", c.Inicio)
	}
	if len(avisos) == 0 {
		t.Error("deducir el inicio debería dejar aviso")
	}
}
