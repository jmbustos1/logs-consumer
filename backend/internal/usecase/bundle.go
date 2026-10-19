package usecase

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"logs-consumer/internal/domain"
)

// Diagnostico arma el bundle: un solo texto plano, autocontenido, que alcanza para proponer
// un arreglo SIN conocer el proyecto.
//
// La pieza que hace falta y que el pipeline no puede aportar es la número 3: la última corrida
// exitosa. El pipeline solo sabe cómo le fue hoy; el consumer sabe que el martes andaba, y con
// qué números. Eso es la mitad de un diagnóstico.
type Diagnostico struct {
	repo domain.Repositorio
	diag domain.Diagnosticador
}

func NuevoDiagnostico(repo domain.Repositorio, diag domain.Diagnosticador) *Diagnostico {
	return &Diagnostico{repo: repo, diag: diag}
}

func (d *Diagnostico) Bundle(ctx context.Context, ref domain.Ref) (string, error) {
	c, err := d.repo.LeerCorrida(ctx, ref)
	if err != nil {
		return "", err
	}
	var b strings.Builder

	res := c.Resumen()
	fmt.Fprintf(&b, "# Corrida %s de %s/%s\n\n", c.RunID, c.Proyecto, c.Pipeline)
	fmt.Fprintf(&b, "- estado: **%s**\n- entorno: %s (host %s)\n- versión: %s\n", c.Estado, c.Entorno, c.Host, c.Version)
	fmt.Fprintf(&b, "- inicio: %s (hace %s)\n- duró: %.1f s\n",
		c.Inicio.Format(time.RFC3339), duracionLegible(time.Since(c.Inicio)), c.Segundos)
	if res.EtapaFallo != "" {
		fmt.Fprintf(&b, "- **murió en la etapa `%s`**: %s\n", res.EtapaFallo, res.Error)
	} else if res.Error != "" {
		fmt.Fprintf(&b, "- **error**: %s\n", res.Error)
	}
	if len(c.Contadores) > 0 {
		fmt.Fprintf(&b, "- contadores: %s\n", mapaLegible(c.Contadores))
	}

	b.WriteString("\n## Etapas\n\n```\n")
	for _, e := range c.Etapas() {
		marca := e.Estado
		if marca == "" {
			marca = "-"
		}
		fmt.Fprintf(&b, "%-8s %-24s %8.0f ms  %s\n", marca, e.Nombre, e.MS, e.Error)
	}
	b.WriteString("```\n")

	b.WriteString("\n## Eventos de la corrida\n\n```jsonl\n")
	for _, ev := range c.Eventos {
		// La etapa se muestra explícita: sin ella, `stage.start` y `stage.end` son líneas
		// mudas y se pierde justamente el esqueleto de la ejecución.
		etapa := ev.Etapa
		if etapa != "" {
			etapa = "[" + etapa + "] "
		}
		fmt.Fprintf(&b, "%s %-5s %-22s %s%s\n", ev.TS.Format("15:04:05"), ev.Nivel, ev.Ev, etapa, mapaLegible(ev.Campos))
	}
	b.WriteString("```\n")

	// La comparación con lo que sí funcionaba. Sin esto, quien lea el bundle no puede
	// distinguir "esto nunca anduvo" de "esto se rompió ayer", que son dos problemas distintos.
	if previa, ok := d.ultimaBuena(ctx, c); ok {
		b.WriteString("\n## La última corrida que SÍ funcionó\n\n")
		fmt.Fprintf(&b, "`%s`, %s (hace %s), estado `%s`, %.1f s\n",
			previa.RunID, previa.Inicio.Format(time.RFC3339),
			duracionLegible(time.Since(previa.Inicio)), previa.Estado, previa.Segundos)
		if len(previa.Contadores) > 0 {
			fmt.Fprintf(&b, "\ncontadores de esa corrida: %s\n", mapaLegible(previa.Contadores))
		}
	} else {
		b.WriteString("\n## La última corrida que SÍ funcionó\n\nNo hay ninguna registrada: este pipeline no tiene corridas exitosas anteriores en el consumer.\n")
	}

	if len(c.Contexto) > 0 {
		b.WriteString("\n## Contexto del pipeline\n\n")
		b.WriteString("Lo mandó el propio pipeline: su código y los documentos que explican lo que el código no dice.\n")
		for _, a := range c.Contexto {
			if a.Contenido == "" {
				fmt.Fprintf(&b, "\n- `%s` (%d bytes, contenido no adjunto)\n", a.Archivo, a.Bytes)
				continue
			}
			// Cerca mas larga que cualquiera que traiga el archivo. Los planes y los TRAMPAS.md
			// de los pipelines estan llenos de bloques de codigo; con una cerca de tres, el
			// primero que aparece cierra la nuestra y el resto del bundle se lee como prosa.
			cerca := cercaPara(a.Contenido)
			fmt.Fprintf(&b, "\n### %s\n\n%s\n%s\n%s\n", a.Archivo, cerca, a.Contenido, cerca)
		}
	} else {
		b.WriteString("\n## Contexto del pipeline\n\nEl sobre no trajo `contexto`. Sin el código no se puede proponer un parche concreto.\n")
	}

	b.WriteString(`
## Qué se te pide

Diagnosticá por qué falló esta corrida y proponé un arreglo.

- Trabajá con la evidencia de arriba. Si no alcanza, decí qué falta en vez de suponerlo.
- Mostrá el arreglo como un diff sobre los archivos del contexto.
- NO apliques nada: esto se revisa antes de tocar el pipeline.
- Distinguí la causa del síntoma. Si el fallo es un cambio río arriba (la fuente cambió de
  formato, una ruta pública dejó de responder), decilo: no todo se arregla en el código.
- Si el estado es ` + "`sin_novedad`" + `, ojo: NO es un fallo. Significa que la fuente no
  republicó nada, y el pipeline salió con código 0 a propósito.
`)
	return b.String(), nil
}

// ultimaBuena busca hacia atrás la última corrida que NO falló.
//
// Ojo con `sin_novedad`: es una corrida que funcionó perfectamente y salió con código 0 —la
// fuente no republicó nada—. Buscar solo `ok` haría que un pipeline mensual sano pareciera no
// haber funcionado nunca, que es justo la confusión contra la que el estado existe.
func (d *Diagnostico) ultimaBuena(ctx context.Context, c domain.Corrida) (domain.ResumenCorrida, bool) {
	rs, err := d.repo.ListarCorridas(ctx, domain.Filtro{
		Proyecto: c.Proyecto, Pipeline: c.Pipeline, Hasta: c.RunID, Limite: 20,
	})
	if err != nil {
		return domain.ResumenCorrida{}, false
	}
	for _, r := range rs {
		if r.RunID != c.RunID && !r.Estado.EsFallo() {
			return r, true
		}
	}
	return domain.ResumenCorrida{}, false
}

// Proponer llama al modelo. Viene apagado: sin LC_DIAGNOSTICO=on y una llave, devuelve
// ErrDesactivado y el panel sigue ofreciendo el bundle para copiar, que es gratis.
func (d *Diagnostico) Proponer(ctx context.Context, ref domain.Ref) (domain.Propuesta, error) {
	if d.diag == nil || !d.diag.Activo() {
		return domain.Propuesta{}, domain.ErrDesactivado
	}
	bundle, err := d.Bundle(ctx, ref)
	if err != nil {
		return domain.Propuesta{}, err
	}
	return d.diag.Proponer(ctx, bundle)
}

func mapaLegible(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	claves := make([]string, 0, len(m))
	for k := range m {
		claves = append(claves, k)
	}
	// orden estable: dos bundles de la misma corrida deben ser idénticos
	for i := 1; i < len(claves); i++ {
		for j := i; j > 0 && claves[j] < claves[j-1]; j-- {
			claves[j], claves[j-1] = claves[j-1], claves[j]
		}
	}
	partes := make([]string, 0, len(claves))
	for _, k := range claves {
		partes = append(partes, fmt.Sprintf("%s=%s", k, valorLegible(m[k])))
	}
	return strings.Join(partes, " ")
}

// valorLegible evita que 1357982 se imprima como 1.357982e+06. JSON no distingue enteros de
// flotantes, pero un contador de filas leído en notación científica es ilegible justo cuando
// hay que comparar dos corridas.
// cercaPara devuelve una cerca de backticks mas larga que la mas larga del contenido.
func cercaPara(contenido string) string {
	mayor, actual := 2, 0
	for _, r := range contenido {
		if r == '`' {
			actual++
			if actual > mayor {
				mayor = actual
			}
		} else {
			actual = 0
		}
	}
	return strings.Repeat("`", mayor+1)
}

func valorLegible(v any) string {
	if f, ok := v.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return fmt.Sprintf("%v", v)
}

func duracionLegible(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%.0f s", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%.0f min", d.Minutes())
	case d < 48*time.Hour:
		return fmt.Sprintf("%.1f h", d.Hours())
	default:
		return fmt.Sprintf("%.0f días", d.Hours()/24)
	}
}
