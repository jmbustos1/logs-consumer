// El servidor. Arranca en local con ListenAndServe; el día que corra en Lambda, el adaptador
// envuelve este mismo http.Handler y no cambia una línea de lo de arriba.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"logs-consumer/config"
	"logs-consumer/internal/adapter/almacen"
	"logs-consumer/internal/adapter/almacen/disco"
	s3almacen "logs-consumer/internal/adapter/almacen/s3"
	"logs-consumer/internal/adapter/claude"
	"logs-consumer/internal/adapter/notificador"
	"logs-consumer/internal/adapter/secretos"
	"logs-consumer/internal/domain"
	"logs-consumer/internal/handler"
	"logs-consumer/internal/logjson"
	"logs-consumer/internal/router"
	"logs-consumer/internal/usecase"
)

func main() {
	cfg := config.Cargar()
	log := logjson.Nuevo(cfg.Entorno, cfg.Version)

	// Los secretos, si están en SSM. Que falten no impide arrancar —el servicio avisa que hay
	// rutas abiertas—, porque un panel que no levanta es peor que uno que levanta gritando.
	if cfg.SSMPrefijo != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		vals, err := secretos.Resolver(ctx, cfg.SSMPrefijo, "token_emisor", "token_lector", "anthropic_api_key")
		cancel()
		if err != nil {
			log.Warn("secretos.no_resueltos", "prefijo", cfg.SSMPrefijo, "error", err)
		} else {
			if v := vals["token_emisor"]; v != "" {
				cfg.TokenEmisor = v
			}
			if v := vals["token_lector"]; v != "" {
				cfg.TokenLector = v
			}
			if v := vals["anthropic_api_key"]; v != "" {
				cfg.AnthropicKey = v
			}
			log.Info("secretos.resueltos", "prefijo", cfg.SSMPrefijo, "encontrados", len(vals))
		}
	}

	// El almacén se elige por configuración. Los dos usan el mismo layout de claves, así que
	// pasar de local a AWS es cambiar una variable de entorno, no reescribir nada.
	var alm domain.Almacen
	var err error
	switch cfg.Almacen {
	case "s3":
		alm, err = s3almacen.Nuevo(context.Background(), cfg.Bucket)
	default:
		alm, err = disco.Nuevo(cfg.DatosDir)
	}
	if err != nil {
		log.Error("arranque.fallido", "motivo", "no se pudo abrir el almacén", "almacen", cfg.Almacen, "error", err)
		os.Exit(1)
	}
	repo := almacen.NuevoRepo(alm)

	diagCliente := claude.Nuevo(cfg.Diagnostico, cfg.AnthropicKey, cfg.Modelo, cfg.MaxTokens)
	_ = notificador.NuevoSilencioso(log) // el enchufe de alertas: todavía sin canal

	h := router.Nuevo(router.Deps{
		Cfg: cfg,
		Log: log,
		Corridas: handler.NuevoCorridas(
			usecase.NuevaIngesta(repo, log),
			usecase.NuevaConsulta(repo),
			usecase.NuevoDiagnostico(repo, diagCliente),
			cfg.MaxBytes, log),
		Pipelines: handler.NuevoPipelines(usecase.NuevaConsulta(repo), usecase.NuevoVigia(repo), log),
		Servicio:  &handler.Servicio{Version: cfg.Version, Entorno: cfg.Entorno, Arranco: time.Now()},
	})

	// Que quede escrito con qué configuración arrancó: la mitad de los "no anda" se explican
	// con una variable que no estaba puesta.
	log.Info("arranque",
		"puerto", cfg.Puerto, "almacen", cfg.Almacen, "datos", cfg.DatosDir,
		"token_emisor", cfg.TokenEmisor != "", "token_lector", cfg.TokenLector != "",
		"diagnostico", diagCliente.Activo(), "modelo", cfg.Modelo, "max_bytes", cfg.MaxBytes)
	if cfg.TokenEmisor == "" || cfg.TokenLector == "" {
		log.Warn("arranque.sin_token",
			"detalle", "hay rutas abiertas: definí LC_TOKEN_EMISOR y LC_TOKEN_LECTOR antes de exponer esto")
	}
	if cfg.Diagnostico && !diagCliente.Activo() {
		log.Warn("arranque.diagnostico_sin_llave",
			"detalle", "LC_DIAGNOSTICO=on pero ANTHROPIC_API_KEY está vacía: queda apagado")
	}

	// En Lambda no hay ListenAndServe: el runtime entrega eventos. El adaptador envuelve el
	// MISMO http.Handler, así que no hay una segunda versión de las rutas que se pueda
	// desincronizar de esta.
	if cfg.Entorno == "lambda" {
		log.Info("arranque.lambda", "detalle", "sirviendo por Function URL")
		lambda.Start(httpadapter.NewV2(h).ProxyWithContext)
		return
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// Generoso a propósito: un sobre con contexto inline puede pesar decenas de KB y
		// llegar desde una tarea que acaba de morir en una red lenta.
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	idle := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("cierre.forzado", "error", err)
		}
		close(idle)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("servidor.caido", "error", err)
		os.Exit(1)
	}
	<-idle
	log.Info("cierre.limpio")
}
