'use client'

import Link from 'next/link'

// El bloque de marca de AETERN: el glifo, la barra vertical del acento, el nombre espaciado y
// la bajada en versalitas. Calcado de brand/firma-aetern.html.
export default function Marca() {
  return (
    <header className="marca">
      <span className="glifo">&#8743;</span>
      <div>
        <div className="nombre">AETERN</div>
        <div className="bajada">Pipelines</div>
      </div>
      <nav>
        <Link href="/">Panel</Link>
        <Link href="/salud/">Salud</Link>
      </nav>
    </header>
  )
}
