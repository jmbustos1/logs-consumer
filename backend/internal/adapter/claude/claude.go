// Package claude implementa el Diagnosticador contra la API de Claude.
//
// Está escrito y probado, pero VIENE APAGADO: hace falta LC_DIAGNOSTICO=on y una llave. El
// camino por defecto sigue siendo copiar el bundle y pegarlo a mano, que es gratis y no
// depende de que un servicio externo esté arriba.
//
// Tres reglas, las mismas que el envio.py de los pipelines:
//  1. nunca tumba una petición: si la API falla, el bundle sigue disponible;
//  2. una sola llamada por invocación, nunca automática — se dispara con un clic;
//  3. el bundle local es la fuente de verdad; la propuesta es una opinión sobre él.
package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"logs-consumer/internal/domain"
)

// Precios por millón de tokens (USD), API de Anthropic. Mismo criterio que lids/log.py: si
// una corrida va a gastar plata, que quede escrito cuánto.
var precios = map[string][2]float64{
	"claude-fable-5":    {10.00, 50.00},
	"claude-opus-5":     {5.00, 25.00},
	"claude-opus-4-8":   {5.00, 25.00},
	"claude-opus-4-7":   {5.00, 25.00},
	"claude-opus-4-6":   {5.00, 25.00},
	"claude-sonnet-5":   {2.00, 10.00},
	"claude-sonnet-4-6": {3.00, 15.00},
	"claude-haiku-4-5":  {1.00, 5.00},
}

const sistema = `Sos un ingeniero de datos que diagnostica fallos de pipelines ETL a partir de
la evidencia de una sola corrida. Quien te lee no tiene el repositorio a mano: todo lo que
sabés del proyecto está en el mensaje.

Respondé en español, en markdown, con esta estructura:

1. **Qué pasó** — en dos o tres frases, la causa, no el síntoma.
2. **Evidencia** — las líneas concretas del log o del código que sostienen esa conclusión.
3. **Arreglo propuesto** — como diff sobre los archivos del contexto.
4. **Qué no sé** — lo que haría falta verificar y no está en la evidencia.

Reglas:
- No inventes archivos, funciones ni campos que no estén en el contexto.
- Si la evidencia no alcanza para una causa, decilo en vez de elegir la más plausible.
- No propongas reintentar como arreglo salvo que el error sea claramente transitorio.
- Si el fallo viene de un cambio río arriba, decilo: eso no se arregla en el código.`

type Cliente struct {
	c         anthropic.Client
	modelo    string
	maxTokens int
	activo    bool
}

func Nuevo(activar bool, llave, modelo string, maxTokens int) *Cliente {
	cl := &Cliente{modelo: modelo, maxTokens: maxTokens}
	// Sin llave no se activa aunque se pida: fallar acá es mejor que fallar en el primer clic.
	if !activar || llave == "" {
		return cl
	}
	cl.c = anthropic.NewClient(option.WithAPIKey(llave))
	cl.activo = true
	return cl
}

func (c *Cliente) Activo() bool { return c.activo }

func (c *Cliente) Proponer(ctx context.Context, bundle string) (domain.Propuesta, error) {
	if !c.activo {
		return domain.Propuesta{}, domain.ErrDesactivado
	}
	t0 := time.Now()

	// El modelo por defecto es claude-opus-5. En Opus 5 el pensamiento adaptativo está
	// encendido de fábrica: no hace falta configurarlo, y configurarlo mal costaría un 400.
	resp, err := c.c.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.modelo),
		MaxTokens: int64(c.maxTokens),
		System:    []anthropic.TextBlockParam{{Text: sistema}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(bundle)),
		},
	})
	if err != nil {
		return domain.Propuesta{}, fmt.Errorf("llamando a la API de Claude: %w", err)
	}

	var texto strings.Builder
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			texto.WriteString(t.Text)
		}
	}
	if resp.StopReason == "refusal" {
		return domain.Propuesta{}, fmt.Errorf("el modelo declinó responder (stop_reason=refusal)")
	}

	ent, sal := int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens)
	return domain.Propuesta{
		Modelo:    c.modelo,
		Texto:     texto.String(),
		TokensEnt: ent,
		TokensSal: sal,
		CostoUSD:  costo(c.modelo, ent, sal),
		Segundos:  time.Since(t0).Seconds(),
	}, nil
}

func costo(modelo string, entrada, salida int) float64 {
	p, ok := precios[modelo]
	if !ok {
		return 0 // un modelo que no está en la tabla reporta 0, no un número inventado
	}
	return float64(entrada)/1e6*p[0] + float64(salida)/1e6*p[1]
}
