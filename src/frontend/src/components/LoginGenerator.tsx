import { useState, useEffect, useCallback, useRef } from 'react'

const apiBaseEnv = import.meta.env.VITE_API_BASE_URL ?? '/api'
const API_BASE = apiBaseEnv.replace(/\/$/, '')

interface Props {
  onChange: (login: string) => void
}

type AvailStatus = 'idle' | 'checking' | 'available' | 'taken' | 'error'

// Кастомный dropdown: триггер — просто текст, список — стилизованный попап
function InlineSelect({
  value, options, onChange, colorClass,
}: {
  value: string
  options: string[]
  onChange: (v: string) => void
  colorClass: string
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className={`${colorClass} font-semibold text-base leading-none underline decoration-dotted underline-offset-2 cursor-pointer outline-none hover:opacity-75 transition-opacity`}
      >
        {value || '…'}
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 bg-white border border-zinc-200 rounded-xl shadow-lg overflow-y-auto max-h-52 min-w-[7rem] py-1">
          {options.map(opt => (
            <button
              key={opt}
              type="button"
              onClick={() => { onChange(opt); setOpen(false) }}
              className={`w-full text-left px-3 py-1.5 text-sm transition-colors ${
                opt === value
                  ? 'font-semibold text-orange-600 bg-orange-50'
                  : 'text-zinc-700 hover:bg-zinc-50 hover:text-zinc-900'
              }`}
            >
              {opt}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export default function LoginGenerator({ onChange }: Props) {
  const [adjectives, setAdjectives] = useState<string[]>([])
  const [nouns, setNouns] = useState<string[]>([])
  const [adj, setAdj] = useState('')
  const [noun, setNoun] = useState('')
  const [num, setNum] = useState('42')
  const [numInput, setNumInput] = useState('42')
  const [status, setStatus] = useState<AvailStatus>('idle')
  const [isGenerating, setIsGenerating] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    Promise.all([
      fetch(`${API_BASE}/users/login-words/adjectives`).then(r => r.json()),
      fetch(`${API_BASE}/users/login-words/nouns`).then(r => r.json()),
    ]).then(([adjs, ns]) => {
      setAdjectives(adjs.map((w: { word: string }) => w.word))
      setNouns(ns.map((w: { word: string }) => w.word))
    }).catch(() => {})
  }, [])

  const generate = useCallback(async () => {
    setIsGenerating(true)
    try {
      const res = await fetch(`${API_BASE}/users/generate-random-login`)
      if (!res.ok) return
      const { login } = await res.json()
      const m = (login as string).match(/^([A-Z][a-z]+)([A-Z][a-z]+)(\d+)$/)
      if (m) { setAdj(m[1]); setNoun(m[2]); setNum(m[3]); setNumInput(m[3]) }
    } catch {}
    finally { setIsGenerating(false) }
  }, [])

  useEffect(() => { generate() }, [generate])

  const login = `${adj}${noun}${num}`
  useEffect(() => { onChange(login) }, [login, onChange])

  useEffect(() => {
    if (!login || login.length < 3) { setStatus('idle'); return }
    setStatus('checking')
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      try {
        const res = await fetch(`${API_BASE}/users/login-words/check-availability`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ login }),
        })
        const data = await res.json()
        setStatus(res.ok ? (data.available ? 'available' : 'taken') : 'error')
      } catch { setStatus('error') }
    }, 400)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [login])

  const handleNum = (v: string) => {
    const clean = v.replace(/\D/g, '').slice(0, 3)
    setNumInput(clean)
    if (!clean) { setNum(''); return }
    const n = parseInt(clean, 10)
    setNum(n >= 1 && n <= 999 ? clean : '')
  }

  const statusEl = () => {
    if (status === 'checking') return <span className="text-xs text-zinc-400">проверяем…</span>
    if (status === 'available') return <span className="text-xs text-green-600 font-medium">✓ свободен</span>
    if (status === 'taken') return <span className="text-xs text-rose-500 font-medium">✗ занят — нажмите 🎲</span>
    return null
  }

  return (
    <div className="grid gap-1">
      <div className="flex items-center px-4 py-3 rounded-xl border border-zinc-200 bg-white hover:border-orange-300 focus-within:border-orange-500 focus-within:ring-2 focus-within:ring-orange-100 transition-all duration-200">
        {/* Три части без пробелов — единое слово */}
        <InlineSelect value={adj} options={adjectives} onChange={setAdj} colorClass="text-orange-500" />
        <InlineSelect value={noun} options={nouns} onChange={setNoun} colorClass="text-rose-500" />
        <input
          type="text"
          inputMode="numeric"
          value={numInput}
          onChange={e => handleNum(e.target.value)}
          className={`font-semibold text-base leading-none bg-transparent outline-none border-none underline decoration-dotted underline-offset-2 text-left p-0 ${
            !numInput ? 'text-rose-400 decoration-rose-300 placeholder-rose-300' : 'text-amber-500 decoration-amber-300'
          }`}
          style={{ width: `${Math.max(numInput.length || 3, 3)}ch` }}
          placeholder="   "
        />

        <button
          type="button"
          onClick={generate}
          disabled={isGenerating}
          title="Случайный логин"
          className="ml-auto text-xl disabled:opacity-40 hover:scale-110 active:scale-95 transition-transform duration-150 leading-none"
        >
          {isGenerating ? '⏳' : '🎲'}
        </button>
      </div>

      <div className="pl-1 h-4">{statusEl()}</div>
    </div>
  )
}
