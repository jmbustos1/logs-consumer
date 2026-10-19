// Package middleware: lo transversal a toda petición. Cada pieza hace una cosa y se compone
// en el router, para que leer router.go alcance para saber qué protege qué.
package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Cadena func(http.Handler) http.Handler

func Componer(h http.Handler, cs ...Cadena) http.Handler {
	for i := len(cs) - 1; i >= 0; i-- {
		h = cs[i](h)
	}
	return h
}

// Recuperar: un panic en una petición no puede llevarse el proceso. Un consumer de logs que
// se cae por un sobre raro es peor que inútil, porque justo se cae cuando algo anda mal.
func Recuperar(log *slog.Logger) Cadena {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					log.Error("http.panic", "ruta", r.URL.Path, "valor", v)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"error interno"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type escritor struct {
	http.ResponseWriter
	codigo int
	bytes  int
}

func (e *escritor) WriteHeader(c int) { e.codigo = c; e.ResponseWriter.WriteHeader(c) }
func (e *escritor) Write(b []byte) (int, error) {
	n, err := e.ResponseWriter.Write(b)
	e.bytes += n
	return n, err
}

// Registrar deja una línea JSON por petición, igual que los pipelines dejan una por evento.
func Registrar(log *slog.Logger) Cadena {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			e := &escritor{ResponseWriter: w, codigo: 200}
			next.ServeHTTP(e, r)
			nivel := slog.LevelInfo
			if e.codigo >= 500 {
				nivel = slog.LevelError
			}
			log.Log(r.Context(), nivel, "http",
				"metodo", r.Method, "ruta", r.URL.Path, "codigo", e.codigo,
				"ms", float64(time.Since(t0).Microseconds())/1000, "bytes", e.bytes)
		})
	}
}

// RequiereToken protege una ruta con un Bearer. Dos tokens distintos —emisor y lector— para
// que filtrar el token de un pipeline no abra la lectura de todos los demás.
//
// Token vacío = ruta abierta. Es lo cómodo en local y una decisión explícita en producción:
// el arranque avisa cuando queda así.
func RequiereToken(esperado, para string) Cadena {
	return func(next http.Handler) http.Handler {
		if esperado == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dado := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			// Comparación en tiempo constante: comparar con == filtra el prefijo correcto
			// por el tiempo que tarda en fallar.
			if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(dado)), []byte(esperado)) != 1 {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"no autorizado","detalle":"falta el Bearer de ` + para + `"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CORS(origenes []string) Cadena {
	permitido := map[string]bool{}
	for _, o := range origenes {
		permitido[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origen := r.Header.Get("Origin")
			if permitido[origen] || permitido["*"] {
				if permitido["*"] {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origen)
				}
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(86400))
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
