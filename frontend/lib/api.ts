// La costura con el backend: una sola URL base y un token. Cambiar de Lambda a Fargate, o de
// local a producción, es cambiar esta variable — no tocar el front.
const BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8787/api/v1'

const CLAVE_TOKEN = 'lc_token_lector'

export function token(): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(CLAVE_TOKEN) || ''
}
export function guardarToken(t: string) {
  window.localStorage.setItem(CLAVE_TOKEN, t)
}

async function pedir<T>(ruta: string, init?: RequestInit): Promise<T> {
  const t = token()
  const r = await fetch(BASE + ruta, {
    ...init,
    headers: { ...(init?.headers || {}), ...(t ? { Authorization: `Bearer ${t}` } : {}) },
  })
  if (!r.ok) {
    let detalle = `HTTP ${r.status}`
    try {
      const j = await r.json()
      detalle = j.detalle || j.error || detalle
    } catch {}
    throw new Error(detalle)
  }
  const tipo = r.headers.get('content-type') || ''
  return (tipo.includes('json') ? await r.json() : await r.text()) as T
}

// ── tipos, calcados del dominio del backend ──────────────────────────────────
export type Estado = 'ok' | 'sin_novedad' | 'error' | 'en_silencio' | string

export interface ResumenCorrida {
  proyecto: string; pipeline: string; run_id: string
  entorno?: string; version?: string; host?: string; comando?: string
  status: Estado; inicio: string; segundos: number
  contadores?: Record<string, unknown>
  num_eventos: number; num_errores: number; num_etapas: number
  error?: string; etapa_fallo?: string
  tiene_contexto: boolean; bytes_sobre: number; recibido: string
}

export interface Evento {
  ts: string; lvl: string; ev: string; etapa?: string
  campos?: Record<string, unknown>
}

export interface Etapa {
  nombre: string; ms: number; estado: string; error?: string
  eventos: Evento[]; salida: string[]
}

export interface Pipeline {
  proyecto: string; pipeline: string; cadencia_h: number
  evento_novedad?: string; descripcion?: string
  ultima?: ResumenCorrida; corridas: number; actualizado: string
  estado_actual: Estado; en_silencio: boolean; desde_ultima?: string
}

export interface Vista {
  corrida: ResumenCorrida & { contexto?: { archivo: string; bytes: number }[] }
  etapas: Etapa[]
  novedad?: Evento
  pipeline: Pipeline
}

export interface Salud {
  ahora: string; sano: boolean
  pipelines: Pipeline[]; silencios: Pipeline[]; fallos: ResumenCorrida[]
}

// ── llamadas ─────────────────────────────────────────────────────────────────
export const pipelines = () => pedir<{ pipelines: Pipeline[] }>('/pipelines')

export const corridas = (q: Record<string, string | number | undefined>) => {
  const p = new URLSearchParams()
  Object.entries(q).forEach(([k, v]) => { if (v !== undefined && v !== '') p.set(k, String(v)) })
  return pedir<{ corridas: ResumenCorrida[]; total: number }>('/corridas?' + p.toString())
}

export const vista = (runID: string, nivel?: string) =>
  pedir<Vista>(`/corridas/${runID}/vista` + (nivel ? `?nivel=${nivel}` : ''))

export const bundle = (runID: string) => pedir<string>(`/corridas/${runID}/bundle`)

export const salud = () => pedir<Salud>('/salud')

export const diagnosticar = (runID: string) =>
  pedir<{ modelo: string; texto: string; costo_usd: number }>(`/corridas/${runID}/diagnostico`, { method: 'POST' })

// ── formato ──────────────────────────────────────────────────────────────────
export function duracion(seg: number): string {
  if (seg < 60) return `${seg.toFixed(1)} s`
  if (seg < 3600) return `${(seg / 60).toFixed(1)} min`
  return `${(seg / 3600).toFixed(1)} h`
}

export function hace(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  const m = ms / 60000
  if (m < 1) return 'recién'
  if (m < 60) return `hace ${Math.round(m)} min`
  if (m < 48 * 60) return `hace ${Math.round(m / 60)} h`
  return `hace ${Math.round(m / 1440)} días`
}

export function ms(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)} s`
  return `${Math.round(n)} ms`
}

// El estado no tiene una etiqueta obvia en castellano y la diferencia importa: sin_novedad NO
// es un fallo, es que la fuente no republicó nada.
export const etiquetaEstado: Record<string, string> = {
  ok: 'ok',
  sin_novedad: 'sin novedad',
  error: 'error',
  en_silencio: 'en silencio',
}
