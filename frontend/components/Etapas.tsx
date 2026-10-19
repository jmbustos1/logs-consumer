'use client'

import { useState } from 'react'
import { ms, type Etapa } from '@/lib/api'

// La vista de una etapa. Se despliega sola cuando falló: si algo se rompió, lo último que
// querés es tener que hacer un clic para ver por qué.
function Fila({ etapa, maxMS }: { etapa: Etapa; maxMS: number }) {
  const [abierta, setAbierta] = useState(etapa.estado === 'error')
  const hayCuerpo = etapa.eventos.length > 0 || etapa.salida.length > 0 || !!etapa.error
  const ancho = maxMS > 0 ? Math.max(2, (etapa.ms / maxMS) * 100) : 0

  return (
    <div className="etapa">
      <div className="etapa-cab" onClick={() => hayCuerpo && setAbierta(!abierta)}>
        <span className="flecha">{hayCuerpo ? (abierta ? '▾' : '▸') : '·'}</span>
        <span className="etapa-nombre">
          {etapa.nombre}
          {etapa.salida.length > 0 && (
            <span className="tenue" style={{ fontSize: 11, marginLeft: 8 }}>
              {etapa.salida.length} líneas
            </span>
          )}
        </span>
        <span className="etapa-ms">{etapa.ms > 0 ? ms(etapa.ms) : ''}</span>
        <span className={`barra ${etapa.estado === 'error' ? 'falla' : ''}`}>
          <i style={{ width: `${ancho}%` }} />
        </span>
      </div>

      {abierta && (
        <div className="etapa-cuerpo">
          {etapa.error && <div className="error-caja">{etapa.error}</div>}

          {etapa.eventos.map((ev, i) => (
            <div key={i} className={`evento nivel-${ev.lvl}`}>
              <span className="hora">{ev.ts ? new Date(ev.ts).toISOString().slice(11, 19) : '--:--:--'}</span>
              <span className="nombre">{ev.ev}</span>
              <span className="campos">
                {Object.entries(ev.campos || {})
                  .map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
                  .join('  ')}
              </span>
            </div>
          ))}

          {/* La salida cruda del subproceso. Es lo que en la máquina se vería por pantalla, y
              es la mitad del valor de esta pantalla. */}
          {etapa.salida.length > 0 && (
            <div className="consola">{etapa.salida.join('\n')}</div>
          )}
        </div>
      )}
    </div>
  )
}

export default function Etapas({ etapas }: { etapas: Etapa[] }) {
  const maxMS = Math.max(0, ...etapas.map(e => e.ms))
  return <div>{etapas.map(e => <Fila key={e.nombre} etapa={e} maxMS={maxMS} />)}</div>
}
