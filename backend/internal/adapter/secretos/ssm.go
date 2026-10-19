// Package secretos resuelve los tokens desde SSM Parameter Store cuando el servicio corre en
// AWS.
//
// Por qué no vienen como variables de entorno de la task definition: los valores tendrían que
// pasar por Terraform y quedarían escritos en el tfstate, que es texto plano. Terraform crea
// los parámetros vacíos, se cargan con la CLI, y el servicio los lee al arrancar.
package secretos

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Resolver lee /prefijo/<nombre> para cada nombre pedido. Devuelve lo que encontró; los que
// falten quedan fuera del mapa y quien llama decide si eso es un problema.
func Resolver(ctx context.Context, prefijo string, nombres ...string) (map[string]string, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("credenciales de AWS: %w", err)
	}
	c := ssm.NewFromConfig(cfg)

	rutas := make([]string, 0, len(nombres))
	for _, n := range nombres {
		rutas = append(rutas, strings.TrimSuffix(prefijo, "/")+"/"+n)
	}

	r, err := c.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          rutas,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("leyendo SSM %s: %w", prefijo, err)
	}

	out := map[string]string{}
	for _, p := range r.Parameters {
		nombre := aws.ToString(p.Name)
		valor := aws.ToString(p.Value)
		// Terraform los crea con este centinela: un parámetro sin cargar vale lo mismo que
		// uno que no existe, y confundirlos dejaría el servicio "protegido" con la palabra
		// `sin-definir` como token.
		if valor == "" || valor == "sin-definir" {
			continue
		}
		out[nombre[strings.LastIndex(nombre, "/")+1:]] = valor
	}
	return out, nil
}
