'use client'

import { Suspense, useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import Estado from '@/components/Estado'
import { corridas, duracion, hace, type ResumenCorrida } from '@/lib/api'

const ESTADOS = ['', 'ok', 'sin_novedad', 'error']

function Contenido() {
  const params = useSearchParams()
  const proyecto = params.get('proyecto') || ''
  const pipeline = params.get('pipeline') || ''
  const [rs, setRs] = useState<ResumenCorrida[] | null>(null)
  const [filtro, setFiltro] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    setRs(null)
    corridas({ proyecto, pipeline, status: filtro, limite: 100 })
      .then(r => setRs(r.corridas))
      .catch(e => setError(String(e.message || e)))
  }, [proyecto, pipeline, filtro])

  if (error) return <div className="aviso">{error}</div>

  return (
    <>
      <p style={{ fontSize: 12 }}><Link href="/">← Pipelines</Link></p>
      <h1>{pipeline}</h1>
      <p className="tenue mono" style={{ fontSize: 12 }}>{proyecto}</p>

      <div className="controles">
        {ESTADOS.map(e => (
          <button key={e || 'todos'} className={filtro === e ? 'activo' : ''} onClick={() => setFiltro(e)}>
            {e === '' ? 'todas' : e.replace('_', ' ')}
          </button>
        ))}
      </div>

      <div className="tarjeta">
        {!rs ? <p className="tenue">cargando…</p> : rs.length === 0 ? (
          <p className="vacio">Ninguna corrida con ese filtro.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>corrida</th><th>estado</th><th>cuándo</th><th>duró</th>
                <th>etapas</th><th>dónde murió</th>
              </tr>
            </thead>
            <tbody>
              {rs.map(c => (
                <tr key={c.run_id}>
                  <td className="mono">
                    <Link href={`/corrida/?run=${c.run_id}`}>{c.run_id}</Link>
                  </td>
                  <td><Estado estado={c.status} /></td>
                  <td className="tenue">{hace(c.inicio)}</td>
                  <td className="tenue mono">{duracion(c.segundos)}</td>
                  <td className="tenue mono">{c.num_etapas}</td>
                  <td className="mono" style={{ color: c.etapa_fallo ? 'var(--error)' : 'inherit' }}>
                    {c.etapa_fallo || ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  )
}

export default function PaginaPipeline() {
  return (
    <Suspense fallback={<p className="tenue">cargando…</p>}>
      <Contenido />
    </Suspense>
  )
}
