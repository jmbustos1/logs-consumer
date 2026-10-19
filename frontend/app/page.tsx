'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import Estado from '@/components/Estado'
import { pipelines, hace, duracion, type Pipeline } from '@/lib/api'

export default function Panel() {
  const [datos, setDatos] = useState<Pipeline[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    pipelines().then(r => setDatos(r.pipelines)).catch(e => setError(String(e.message || e)))
  }, [])

  if (error) return <div className="aviso">No se pudo hablar con la API: {error}</div>
  if (!datos) return <p className="tenue">cargando…</p>
  if (datos.length === 0)
    return (
      <div className="aviso">
        Todavía no reportó ningún pipeline. Para conectar el primero, seguí <code>INTEGRACION.md</code> o
        pedile el contrato a la propia API: <code>GET /api/v1/contrato</code>.
      </div>
    )

  return (
    <>
      <h1>Pipelines</h1>
      <p className="tenue">Uno por fila, con su última corrida. La cadencia declarada es lo que
        permite notar el silencio: sin ella, un pipeline que dejó de correr no se distingue de uno
        que corrió bien la semana pasada.</p>

      {datos.map(p => (
        <Link key={`${p.proyecto}/${p.pipeline}`}
              href={`/pipeline/?proyecto=${p.proyecto}&pipeline=${p.pipeline}`}
              style={{ color: 'inherit' }}>
          <div className="tarjeta clickable">
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 14, flexWrap: 'wrap' }}>
              <span className="mono" style={{ fontSize: 14, color: 'var(--tinta)', fontWeight: 600 }}>
                {p.pipeline}
              </span>
              <span className="tenue mono" style={{ fontSize: 11.5 }}>{p.proyecto}</span>
              <span style={{ marginLeft: 'auto' }}><Estado estado={p.estado_actual} /></span>
            </div>

            <hr className="hairline" />

            <div style={{ display: 'flex', gap: 26, flexWrap: 'wrap', fontSize: 12.5 }}>
              <span className="tenue">
                {p.ultima ? <>última {hace(p.ultima.inicio)} · {duracion(p.ultima.segundos)}</> : 'sin corridas'}
              </span>
              <span className="tenue">{p.corridas} corridas</span>
              <span className="tenue">
                {p.cadencia_h > 0 ? `cada ${p.cadencia_h} h` : 'sin cadencia declarada'}
              </span>
              {p.ultima?.etapa_fallo && (
                <span style={{ color: 'var(--error)' }} className="mono">
                  murió en {p.ultima.etapa_fallo}
                </span>
              )}
            </div>

            {p.ultima?.contadores && Object.keys(p.ultima.contadores).length > 0 && (
              <div className="contadores" style={{ marginTop: 10 }}>
                {Object.entries(p.ultima.contadores).slice(0, 6).map(([k, v]) => (
                  <span key={k} className="tenue">{k} <b>{String(v)}</b></span>
                ))}
              </div>
            )}
          </div>
        </Link>
      ))}
    </>
  )
}
