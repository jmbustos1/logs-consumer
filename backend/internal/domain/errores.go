package domain

import "errors"

// Errores de dominio. El handler los traduce a códigos HTTP en un solo lugar, así ningún
// caso de uso tiene que saber qué es un 404.
var (
	ErrInvalido        = errors.New("sobre inválido")
	ErrNoEncontrado    = errors.New("no encontrado")
	ErrDemasiadoGrande = errors.New("cuerpo demasiado grande")
	ErrNoAutorizado    = errors.New("no autorizado")
	ErrDesactivado     = errors.New("funcionalidad desactivada")
)
