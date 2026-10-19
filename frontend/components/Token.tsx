'use client'

import { useEffect, useState } from 'react'
import { token, guardarToken } from '@/lib/api'

// El panel no tiene sesiones ni usuarios: un token de lectura, guardado en el navegador.
// Sin esto, un panel recién desplegado responde 401 y no hay dónde escribir la llave — que es
// exactamente el estado en el que quedaría después de un despliegue limpio.
export default function Token() {
  const [guardado, setGuardado] = useState<string | null>(null)
  const [valor, setValor] = useState('')
  const [editando, setEditando] = useState(false)

  useEffect(() => setGuardado(token()), [])

  if (guardado === null) return null // todavía no se leyó el localStorage

  function guardar() {
    guardarToken(valor.trim())
    window.location.reload()
  }

  if (guardado && !editando) {
    return (
      <div style={{ textAlign: 'right', marginTop: -18, marginBottom: 12 }}>
        <button
          onClick={() => setEditando(true)}
          title="Cambiar el token de lectura"
          style={{ border: 0, background: 'none', padding: 0, fontSize: 10.5, letterSpacing: 1.5 }}
          className="tenue mono"
        >
          TOKEN ·{guardado.slice(-4)}
        </button>
      </div>
    )
  }

  return (
    <div className="aviso" style={{ marginBottom: 18 }}>
      <div style={{ marginBottom: 8 }}>
        <b>Token de lectura.</b> Es el que está en SSM como <code>/logs-consumer-api/token_lector</code>.
        Queda guardado en este navegador y no viaja a ningún lado más que a la API.
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <input
          type="password"
          value={valor}
          onChange={e => setValor(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && guardar()}
          placeholder="lcl_…"
          style={{
            flex: 1, fontFamily: 'var(--mono)', fontSize: 12, padding: '6px 10px',
            border: '1px solid var(--borde)', borderRadius: 6, background: 'var(--superficie)',
          }}
        />
        <button className="principal" onClick={guardar}>Guardar</button>
        {guardado && <button onClick={() => setEditando(false)}>Cancelar</button>}
      </div>
    </div>
  )
}
