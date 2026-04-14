import { useState, useEffect, useCallback } from 'react'
import { getProductMetrics, getTechnicalMetrics, getAIMetrics, type ProductMetrics } from '../services/api'

type Tab = 'product' | 'technical' | 'ai'

function MetricCard({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div className="bg-white border border-zinc-200 rounded-xl p-5 shadow-sm">
      <p className="text-xs text-zinc-500 mb-1">{label}</p>
      <p className="text-2xl font-bold text-zinc-900">{value}</p>
      {sub && <p className="text-xs text-zinc-400 mt-1">{sub}</p>}
    </div>
  )
}

function PrometheusValue({ data, label }: { data: unknown; label: string }) {
  const extract = (d: unknown): string => {
    try {
      const r = d as any
      if (!r || r.status !== 'success') return '—'
      const results = r?.data?.result ?? []
      if (results.length === 0) return '0'
      const vals = results.map((item: any) => {
        const v = parseFloat(item?.value?.[1] ?? '0')
        return isNaN(v) ? 0 : v
      })
      const sum = vals.reduce((a: number, b: number) => a + b, 0)
      return sum < 1 ? sum.toFixed(4) : sum.toFixed(2)
    } catch {
      return '—'
    }
  }
  return <MetricCard label={label} value={extract(data)} />
}

// ── Product tab ────────────────────────────────────────────────────────────────
function ProductTab() {
  const [metrics, setMetrics] = useState<ProductMetrics | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [calculatedAt, setCalculatedAt] = useState<Date | null>(null)

  const calculate = () => {
    setLoading(true)
    setError(null)
    getProductMetrics()
      .then(data => {
        setMetrics(data)
        setCalculatedAt(new Date())
      })
      .catch(e => setError(e instanceof Error ? e.message : 'Ошибка'))
      .finally(() => setLoading(false))
  }

  if (!metrics && !loading && !error) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-4">
        <div className="text-4xl">📊</div>
        <p className="text-zinc-500 text-sm">Нажмите кнопку, чтобы рассчитать продуктовые метрики из базы данных</p>
        <button
          onClick={calculate}
          className="px-5 py-2.5 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 transition-colors"
        >
          Рассчитать метрики
        </button>
      </div>
    )
  }

  if (loading) return <p className="text-sm text-zinc-500 py-8 text-center">Расчёт метрик…</p>
  if (error) return (
    <div className="flex flex-col items-center gap-3 py-8">
      <p className="text-sm text-red-600 text-center">{error}</p>
      <button
        onClick={calculate}
        className="px-4 py-2 rounded-lg border border-zinc-300 text-zinc-700 text-sm hover:bg-zinc-50"
      >
        Повторить
      </button>
    </div>
  )
  if (!metrics) return null

  const stars = metrics.avg_rating > 0 ? `★ ${metrics.avg_rating.toFixed(1)}` : '—'
  const byKindEntries = Object.entries(metrics.by_kind ?? {})

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        {calculatedAt && (
          <p className="text-xs text-zinc-400">
            Рассчитано: {calculatedAt.toLocaleTimeString('ru')}
          </p>
        )}
        <button
          onClick={calculate}
          disabled={loading}
          className="ml-auto px-4 py-1.5 rounded-lg border border-zinc-200 text-zinc-600 text-sm hover:bg-zinc-50 transition-colors disabled:opacity-50"
        >
          ↻ Пересчитать
        </button>
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <MetricCard label="Всего сессий" value={metrics.total_sessions} />
        <MetricCard label="Завершены полностью" value={metrics.completed_naturally}
          sub={`${metrics.completion_rate.toFixed(1)}% completion rate`} />
        <MetricCard label="Прерваны досрочно" value={metrics.terminated_early} />
        <MetricCard label="Средний балл" value={`${metrics.avg_score.toFixed(1)}%`} />
      </div>
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
        <MetricCard label="Средняя оценка" value={stars} sub={`${metrics.rated_sessions} оценок`} />
        {byKindEntries.map(([kind, count]) => (
          <MetricCard key={kind} label={`Режим: ${kind}`} value={count} />
        ))}
      </div>
    </div>
  )
}

// ── Technical tab ──────────────────────────────────────────────────────────────
function TechnicalTab() {
  const [metrics, setMetrics] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getTechnicalMetrics()
      .then(setMetrics)
      .catch(e => setError(e instanceof Error ? e.message : 'Ошибка — убедитесь что Prometheus запущен'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-sm text-zinc-500 py-8 text-center">Загрузка из Prometheus…</p>
  if (error) return (
    <div className="py-8 text-center space-y-2">
      <p className="text-sm text-red-600">{error}</p>
      <p className="text-xs text-zinc-400">Prometheus доступен на :9090 при запущенном docker-compose</p>
    </div>
  )
  if (!metrics) return null

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <PrometheusValue data={metrics.request_rate_5m} label="RPS (5m, все сервисы)" />
        <PrometheusValue data={metrics.error_rate_5m} label="Error rate (5m)" />
        <PrometheusValue data={metrics.p95_latency_5m} label="p95 latency, сек (5m)" />
        <PrometheusValue data={metrics.services_up} label="Сервисов online" />
      </div>
      <div className="bg-white border border-zinc-200 rounded-xl p-5 shadow-sm">
        <p className="text-xs font-semibold text-zinc-500 uppercase tracking-wide mb-3">Статус сервисов</p>
        <ServicesUp data={metrics.services_up} />
      </div>
    </div>
  )
}

function ServicesUp({ data }: { data: unknown }) {
  try {
    const r = data as any
    const results: { metric: { job?: string; instance?: string }; value: [number, string] }[] =
      r?.data?.result ?? []
    if (results.length === 0) return <p className="text-xs text-zinc-400">Нет данных</p>
    return (
      <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
        {results.map((item, i) => {
          const name = item.metric?.job ?? item.metric?.instance ?? `service-${i}`
          const up = item.value?.[1] === '1'
          return (
            <div key={i} className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs ${up ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'}`}>
              <span>{up ? '●' : '○'}</span>
              <span className="truncate">{name}</span>
            </div>
          )
        })}
      </div>
    )
  } catch {
    return <p className="text-xs text-zinc-400">Ошибка парсинга</p>
  }
}

// ── AI tab ─────────────────────────────────────────────────────────────────────
function AITab() {
  const [metrics, setMetrics] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getAIMetrics()
      .then(setMetrics)
      .catch(e => setError(e instanceof Error ? e.message : 'Ошибка — убедитесь что Prometheus запущен'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-sm text-zinc-500 py-8 text-center">Загрузка из Prometheus…</p>
  if (error) return (
    <div className="py-8 text-center space-y-2">
      <p className="text-sm text-red-600">{error}</p>
      <p className="text-xs text-zinc-400">Запустите docker-compose чтобы увидеть метрики агента</p>
    </div>
  )
  if (!metrics) return null

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
      <PrometheusValue data={metrics.llm_calls_1h} label="LLM вызовов (1ч)" />
      <PrometheusValue data={metrics.pii_blocks_1h} label="PII блокировок (1ч)" />
      <PrometheusValue data={metrics.active_dialogues} label="Активных диалогов" />
      <PrometheusValue data={metrics.decision_confidence_p50} label="Уверенность агента (p50)" />
      <PrometheusValue data={metrics.processing_p95_5m} label="Время обработки p95, сек" />
      <PrometheusValue data={metrics.message_feedback} label="Оценок сообщений (всего)" />
    </div>
  )
}

// ── Main ───────────────────────────────────────────────────────────────────────
export default function Metrics() {
  const [tab, setTab] = useState<Tab>('product')
  const [lastUpdated, setLastUpdated] = useState(new Date())

  const refresh = useCallback(() => setLastUpdated(new Date()), [])

  const tabs: { id: Tab; label: string; icon: string }[] = [
    { id: 'product', label: 'Продуктовые', icon: '📈' },
    { id: 'technical', label: 'Технические', icon: '⚙️' },
    { id: 'ai', label: 'ИИ-метрики', icon: '🤖' },
  ]

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold text-zinc-900">Метрики</h1>
        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-400">
            Обновлено: {lastUpdated.toLocaleTimeString('ru')}
          </span>
          <button
            onClick={refresh}
            className="px-3 py-1.5 text-sm rounded-lg border border-zinc-200 text-zinc-600 hover:bg-zinc-50 transition-colors"
          >
            ↻ Обновить
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 p-1 bg-zinc-100 rounded-xl mb-6 w-fit">
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              tab === t.id
                ? 'bg-white text-zinc-900 shadow-sm'
                : 'text-zinc-500 hover:text-zinc-700'
            }`}
          >
            <span>{t.icon}</span>
            {t.label}
          </button>
        ))}
      </div>

      {/* Content — key forces remount on refresh */}
      <div key={lastUpdated.getTime()}>
        {tab === 'product' && <ProductTab />}
        {tab === 'technical' && <TechnicalTab />}
        {tab === 'ai' && <AITab />}
      </div>
    </div>
  )
}
