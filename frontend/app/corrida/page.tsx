'use client'

import { Suspense, useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import Estado from '@/components/Estado'
import Etapas from '@/components/Etapas'
import { vista, bundle, duracion, hace, type Vista } from '@/lib/api'

const NIVELES = ['debug', 'info', 'warn', 'error']

function Contenido() {
  const params = useSearchParams()
  const runID = params.get('run') || ''
  const [v, setV] = useState<Vista | null>(null)
  const [error, setError] = useState('')
  const [nivel, setNivel] = useState('debug')
  const [copiado, setCopiado] = useState('')

  useEffect(() => {
    if (!runID) return
    setV(null)
    vista(runID, nivel === 'debug' ? undefined : nivel)
      .then(setV)
      .catch(e => setError(String(e.message || e)))
  }, [runID, nivel])

  async function copiarBundle() {
    try {
      const texto = await bundle(runID)
      await navigator.clipboard.writeText(texto)
      setCopiado(`copiado · ${(texto.length / 1024).toFixed(0)} KB · ~${Math.round(texto.length / 4 / 1000)}k tokens`)
    } catch (e) {
      setCopiado('no se pudo copiar: ' + String(e))
    }
  }

  if (!runID) return <div className="aviso">Falta el parámetro <code>run</code> en la URL.</div>
  if (error) return <div className="aviso">{error}</div>
  if (!v) return <p className="tenue">cargando…</p>

  const c = v.corrida
  return (
    <>
      <p style={{ fontSize: 12 }}>
        <Link href={`/pipeline/?proyecto=${c.proyecto}&pipeline=${c.pipeline}`}>
          ← {c.pipeline}
        </Link>
      </p>

      <div className="tarjeta">
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 14, flexWrap: 'wrap' }}>
          <h1 style={{ margin: 0 }}>{c.pipeline}</h1>
          <span className="mono tenue">{c.run_id}</span>
          <span style={{ marginLeft: 'auto' }}><Estado estado={c.status} /></span>
        </div>

        <hr className="hairline" />

        <div style={{ display: 'flex', gap: 26, flexWrap: 'wrap', fontSize: 12.5 }} className="tenue">
          <span>{duracion(c.segundos)}</span>
          <span>{hace(c.inicio)}</span>
          <span className="mono">{c.entorno}</span>
          {c.host && <span className="mono">{c.host}</span>}
          {c.version && <span className="mono">v{c.version}</span>}
          <span>{(c.bytes_sobre / 1024).toFixed(1)} KB</span>
        </div>

        {c.error && <div className="error-caja">{c.etapa_fallo && <b>{c.etapa_fallo}: </b>}{c.error}</div>}

        {c.contadores && Object.keys(c.contadores).length > 0 && (
          <>
            <hr className="hairline" />
            <div className="contadores">
              {Object.entries(c.contadores).map(([k, val]) => (
                <span key={k} className="tenue">{k} <b>{String(val)}</b></span>
              ))}
            </div>
          </>
        )}
      </div>

      {/* De los eventos de una corrida, casi todos dicen si el pipeline funcionó y uno solo
          dice si pasó algo en el mundo. Ese va arriba y separado. */}
      {v.novedad && (
        <div className="tarjeta" style={{ borderLeft: '3px solid var(--acento)' }}>
          <div className="tenue" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1 }}>
            Novedad · {v.novedad.ev}
          </div>
          <div className="contadores" style={{ marginTop: 8 }}>
            {Object.entries(v.novedad.campos || {}).map(([k, val]) => (
              <span key={k} className="tenue">{k} <b>{typeof val === 'object' ? JSON.stringify(val) : String(val)}</b></span>
            ))}
          </div>
        </div>
      )}

      <div className="controles">
        <span className="tenue" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1 }}>
          Nivel
        </span>
        {NIVELES.map(n => (
          <button key={n} className={nivel === n ? 'activo' : ''} onClick={() => setNivel(n)}>
            {n}
          </button>
        ))}
        <button className="principal" style={{ marginLeft: 'auto' }} onClick={copiarBundle}>
          Copiar bundle
        </button>
        {copiado && <span className="tenue" style={{ fontSize: 12 }}>{copiado}</span>}
      </div>

      <div className="tarjeta">
        <Etapas etapas={v.etapas} />
      </div>

      <p className="tenue" style={{ fontSize: 12 }}>
        El bundle junta esta corrida, la última que sí funcionó y —si el pipeline lo mandó— su
        código y sus documentos, en un texto listo para pegar en una sesión de Claude Code.
        {c.tiene_contexto
          ? ' Esta corrida trae el código adjunto.'
          : ' Esta corrida no trae código adjunto: los pipelines solo lo mandan cuando fallan.'}
      </p>
    </>
  )
}

export default function Corrida() {
  return (
    <Suspense fallback={<p className="tenue">cargando…</p>}>
      <Contenido />
    </Suspense>
  )
}
