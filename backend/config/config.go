// Package config concentra todo lo que depende del despliegue.
//
// Mismo molde que los ETL que este servicio vigila: los valores por defecto viven acá y se
// pisan con variables de entorno, de modo que decidir dónde corre esto no obliga a tocar
// código. Todas las variables llevan el prefijo LC_ salvo ANTHROPIC_API_KEY, que se llama
// igual que en lids-searcher a propósito: es la misma llave y no tiene sentido renombrarla.
package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Entorno string // local | lambda | fargate
	Puerto  string
	Version string

	Almacen  string // disco | s3
	DatosDir string // si Almacen=disco
	Bucket   string // si Almacen=s3

	TokenEmisor string // el que ponen los pipelines en ETL_LOGS_TOKEN
	TokenLector string // el del panel
	MaxBytes    int64

	// Si está definido, los tokens y la llave salen de SSM en vez de las variables de
	// entorno: así los secretos no pasan por Terraform ni quedan en el tfstate.
	SSMPrefijo string

	// El diagnóstico asistido viene apagado. Ver internal/adapter/claude.
	Diagnostico  bool
	AnthropicKey string
	Modelo       string
	MaxTokens    int

	OrigenesCORS []string
}

func Cargar() Config {
	c := Config{
		Entorno: entorno(),
		Puerto:  env("LC_PUERTO", "8080"),
		Version: env("LC_VERSION", "0.1.0"),

		Almacen:  env("LC_ALMACEN", "disco"),
		DatosDir: env("LC_DATOS", "./datos"),
		Bucket:   env("LC_BUCKET", ""),

		SSMPrefijo:  env("LC_SSM_PREFIJO", ""),
		TokenEmisor: env("LC_TOKEN_EMISOR", ""),
		TokenLector: env("LC_TOKEN_LECTOR", ""),
		MaxBytes:    entero64("LC_MAX_BYTES", 8<<20), // 8 MiB: el sobre más gordo medido son 53 KB

		Diagnostico:  env("LC_DIAGNOSTICO", "off") == "on",
		AnthropicKey: env("ANTHROPIC_API_KEY", ""),
		Modelo:       env("LC_MODELO", "claude-opus-5"),
		MaxTokens:    entero("LC_MAX_TOKENS", 4096),

		OrigenesCORS: lista(env("LC_CORS", "http://localhost:3000")),
	}
	return c
}

// entorno se detecta igual que en los ETL: si hay metadata de la plataforma, se usa.
func entorno() string {
	if v := os.Getenv("LC_ENTORNO"); v != "" {
		return v
	}
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		return "lambda"
	}
	if os.Getenv("ECS_CONTAINER_METADATA_URI_V4") != "" {
		return "fargate"
	}
	return "local"
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func entero(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func entero64(k string, def int64) int64 {
	if v, err := strconv.ParseInt(os.Getenv(k), 10, 64); err == nil {
		return v
	}
	return def
}

func lista(s string) []string {
	if s == "" {
		return nil
	}
	partes := strings.Split(s, ",")
	for i := range partes {
		partes[i] = strings.TrimSpace(partes[i])
	}
	return partes
}
