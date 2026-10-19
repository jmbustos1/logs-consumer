// Package disco implementa domain.Almacen sobre el sistema de archivos.
//
// Es el almacén de la versión local, y su árbol es exactamente el mismo que los prefijos de
// S3. Eso no es casualidad: si el layout divergiera, "funciona en local" dejaría de significar
// algo. Además `ls -R datos/` sigue siendo una herramienta de depuración válida.
package disco

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"logs-consumer/internal/domain"
)

type Almacen struct{ base string }

func Nuevo(base string) (*Almacen, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Almacen{base: abs}, nil
}

func (a *Almacen) ruta(clave string) (string, error) {
	limpia := filepath.Clean(filepath.Join(a.base, filepath.FromSlash(clave)))
	// Cinturón además del tirante: las claves se construyen con campos ya validados en
	// domain.seguro(), pero un almacén que escribe donde le digan es un almacén roto.
	if !strings.HasPrefix(limpia, a.base+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: clave fuera del almacén: %q", domain.ErrInvalido, clave)
	}
	return limpia, nil
}

func (a *Almacen) Put(ctx context.Context, clave string, datos []byte) error {
	ruta, err := a.ruta(clave)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	// Escritura atómica: un panel que lee un objeto a medio escribir muestra basura, y peor,
	// la muestra como si fuera un dato.
	tmp, err := os.CreateTemp(filepath.Dir(ruta), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(datos); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), ruta)
}

func (a *Almacen) Get(ctx context.Context, clave string) ([]byte, error) {
	ruta, err := a.ruta(clave)
	if err != nil {
		return nil, err
	}
	datos, err := os.ReadFile(ruta)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", domain.ErrNoEncontrado, clave)
	}
	return datos, err
}

func (a *Almacen) Delete(ctx context.Context, clave string) error {
	ruta, err := a.ruta(clave)
	if err != nil {
		return err
	}
	err = os.Remove(ruta)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (a *Almacen) List(ctx context.Context, prefijo string) ([]domain.Objeto, error) {
	// Se camina desde el directorio más profundo que el prefijo permita, para no recorrer
	// todo el árbol cuando ya se sabe el pipeline.
	dir := prefijo
	if !strings.HasSuffix(prefijo, "/") {
		dir = filepath.ToSlash(filepath.Dir(prefijo))
	}
	raiz, err := a.ruta(strings.TrimSuffix(dir, "/"))
	if err != nil {
		return nil, err
	}

	var out []domain.Objeto
	err = filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // un prefijo sin objetos no es un error, es una lista vacía
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(a.base, ruta)
		if err != nil {
			return err
		}
		clave := filepath.ToSlash(rel)
		if !strings.HasPrefix(clave, prefijo) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, domain.Objeto{Clave: clave, Bytes: info.Size(), Modificado: info.ModTime().UTC()})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}
