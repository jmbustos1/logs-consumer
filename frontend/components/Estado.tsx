import { etiquetaEstado, type Estado as TEstado } from '@/lib/api'

export default function Estado({ estado }: { estado?: TEstado }) {
  const e = estado || 'desconocido'
  return (
    <span className={`estado estado-${e}`}>{etiquetaEstado[e] || e}</span>
  )
}
