// Package almacen decide UNA vez cómo se llaman los objetos, y monta el Repositorio del
// dominio sobre las tres primitivas del puerto Almacen. Disco y S3 comparten este archivo:
// por eso local y producción no pueden divergir en silencio.
package almacen

import (
	"fmt"
	"strings"

	"logs-consumer/internal/domain"
)

// El layout. Dos propiedades lo hacen suficiente sin base de datos:
//
//  1. run_id es AAAAMMDD-HHMMSS-hex, así que ordenar claves alfabéticamente es ordenarlas
//     cronológicamente. "las últimas 50 corridas" = listar y cortar el final.
//  2. el estado va en el NOMBRE del resumen, así que "los fallos de la semana" se responde
//     listando claves, sin abrir un solo objeto.
const (
	PrefSobres    = "sobres/"
	PrefResumen   = "resumen/"
	PrefPipelines = "pipelines/"
)

func ClaveSobre(r domain.Ref) string {
	return fmt.Sprintf("%s%s/%s/%s.json.gz", PrefSobres, r.Proyecto, r.Pipeline, r.RunID)
}

func ClaveResumen(r domain.Ref, estado domain.Estado) string {
	return fmt.Sprintf("%s%s/%s/%s.%s.json", PrefResumen, r.Proyecto, r.Pipeline, r.RunID, estado)
}

// PrefijoResumenRun sirve para encontrar (o borrar) la versión previa de una corrida cuyo
// estado no conocemos: el mismo run_id reenviado con otro status escribiría una clave distinta.
func PrefijoResumenRun(r domain.Ref) string {
	return fmt.Sprintf("%s%s/%s/%s.", PrefResumen, r.Proyecto, r.Pipeline, r.RunID)
}

func PrefijoResumen(proyecto, pipeline string) string {
	switch {
	case proyecto != "" && pipeline != "":
		return fmt.Sprintf("%s%s/%s/", PrefResumen, proyecto, pipeline)
	case proyecto != "":
		return fmt.Sprintf("%s%s/", PrefResumen, proyecto)
	}
	return PrefResumen
}

func ClavePipeline(proyecto, pipeline string) string {
	return fmt.Sprintf("%s%s/%s.json", PrefPipelines, proyecto, pipeline)
}

// LeerClaveResumen deshace la clave. Devuelve la referencia y el estado sin abrir el objeto.
func LeerClaveResumen(clave string) (domain.Ref, domain.Estado, bool) {
	resto, ok := strings.CutPrefix(clave, PrefResumen)
	if !ok {
		return domain.Ref{}, "", false
	}
	partes := strings.Split(resto, "/")
	if len(partes) != 3 {
		return domain.Ref{}, "", false
	}
	nombre, ok := strings.CutSuffix(partes[2], ".json")
	if !ok {
		return domain.Ref{}, "", false
	}
	// run_id.estado — el run_id puede traer puntos, el estado no.
	i := strings.LastIndex(nombre, ".")
	if i < 0 {
		return domain.Ref{}, "", false
	}
	return domain.Ref{Proyecto: partes[0], Pipeline: partes[1], RunID: nombre[:i]}, domain.Estado(nombre[i+1:]), true
}
