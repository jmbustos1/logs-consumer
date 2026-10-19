'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import Estado from '@/components/Estado'
import { salud, hace, type Salud } from '@/lib/api'

export default function PaginaSalud() {
  const [s, setS] = useState<Salud | null>(null)
  const [error, setError] = useState('')

  useEffect(() => { salud().then(setS).catch(e => setError(String(e.message || e))) }, [])

  if (error) return <div className="aviso">{error}</div>
  if (!s) return <p className="tenue">cargando…</p>

  return (
    <>
      <h1>Salud</h1>
      <p className="tenue">
        Dos cosas y nada más: quién dejó de hablar, y a quién le fue mal la última vez. Un
        pipeline que falla manda su sobre y se nota; uno que muere de veras no manda nada, y el
        silencio es la única señal que queda.
      </p>

      <div className="tarjeta">
        <Estado estado={s.sano ? 'ok' : 'error'} />
        <span style={{ marginLeft: 10 }}>
          {s.sano ? 'Todos los pipelines al día.' :
            `${s.silencios.length} en silencio · ${s.fallos.length} con la última corrida fallida`}
        </span>
      </div>

      <h2>En silencio</h2>
      <div className="tarjeta">
        {s.silencios.length === 0 ? (
          <p className="vacio">Ninguno. Los pipelines sin cadencia declarada no se vigilan.</p>
        ) : s.silencios.map(p => (
          <div key={p.pipeline} style={{ padding: '6px 0' }}>
            <span className="mono">{p.pipeline}</span>
            <span className="tenue" style={{ marginLeft: 12 }}>
              debía correr cada {p.cadencia_h} h · última {p.ultima ? hace(p.ultima.inicio) : 'nunca'}
            </span>
          </div>
        ))}
      </div>

      <h2>Última corrida fallida</h2>
      <div className="tarjeta">
        {s.fallos.length === 0 ? <p className="vacio">Ninguna.</p> : (
          <table>
            <thead><tr><th>pipeline</th><th>corrida</th><th>etapa</th><th>error</th></tr></thead>
            <tbody>
              {s.fallos.map(f => (
                <tr key={f.run_id}>
                  <td className="mono">{f.pipeline}</td>
                  <td className="mono"><Link href={`/corrida/?run=${f.run_id}`}>{f.run_id}</Link></td>
                  <td className="mono" style={{ color: 'var(--error)' }}>{f.etapa_fallo}</td>
                  <td className="tenue" style={{ fontSize: 12 }}>{f.error}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  )
}
