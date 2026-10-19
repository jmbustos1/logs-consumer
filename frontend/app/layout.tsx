import type { Metadata } from 'next'
import './globals.css'
import Marca from '@/components/Marca'
import Token from '@/components/Token'

export const metadata: Metadata = {
  title: 'Pipelines · AETERN',
  description: 'Corridas de los pipelines: qué hizo cada etapa y quién dejó de hablar.',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="es">
      <head>
        {/* Inter + Fira Code por link y no por next/font: el sitio es estático y así el build
            no depende de la red. Las pilas de respaldo cubren el caso sin conexión. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&family=Fira+Code:wght@400;500&display=swap"
        />
      </head>
      <body>
        <div className="envoltorio">
          <Marca />
          <Token />
          {children}
        </div>
      </body>
    </html>
  )
}
