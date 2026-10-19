// Package s3 implementa domain.Almacen sobre S3.
//
// Es el gemelo de `disco`: mismas tres primitivas y EXACTAMENTE el mismo layout de claves, que
// vive en el paquete padre. Local y producción no pueden divergir en silencio porque la única
// diferencia entre los dos es dónde terminan los bytes.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"logs-consumer/internal/domain"
)

type Almacen struct {
	c      *s3.Client
	bucket string
}

func Nuevo(ctx context.Context, bucket string) (*Almacen, error) {
	if bucket == "" {
		return nil, errors.New("falta LC_BUCKET")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("cargando credenciales de AWS: %w", err)
	}
	return &Almacen{c: s3.NewFromConfig(cfg), bucket: bucket}, nil
}

func (a *Almacen) Put(ctx context.Context, clave string, datos []byte) error {
	_, err := a.c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(clave),
		Body:   bytes.NewReader(datos),
	})
	return err
}

func (a *Almacen) Get(ctx context.Context, clave string) ([]byte, error) {
	r, err := a.c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(clave),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		// Un objeto que no existe es ErrNoEncontrado del dominio, no un error de AWS: el
		// handler ya sabe traducir eso a un 404 y no tiene por qué conocer S3.
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			return nil, fmt.Errorf("%w: %s", domain.ErrNoEncontrado, clave)
		}
		return nil, err
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func (a *Almacen) Delete(ctx context.Context, clave string) error {
	_, err := a.c.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(clave),
	})
	return err
}

func (a *Almacen) List(ctx context.Context, prefijo string) ([]domain.Objeto, error) {
	var out []domain.Objeto
	p := s3.NewListObjectsV2Paginator(a.c, &s3.ListObjectsV2Input{
		Bucket: aws.String(a.bucket),
		Prefix: aws.String(prefijo),
	})
	for p.HasMorePages() {
		pagina, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range pagina.Contents {
			obj := domain.Objeto{Clave: aws.ToString(o.Key), Bytes: aws.ToInt64(o.Size)}
			if o.LastModified != nil {
				obj.Modificado = o.LastModified.UTC()
			}
			out = append(out, obj)
		}
	}
	return out, nil
}
