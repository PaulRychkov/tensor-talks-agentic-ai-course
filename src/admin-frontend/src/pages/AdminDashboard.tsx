import { useState, useEffect } from 'react'
import { getDrafts, type Draft } from '../services/api'

export default function AdminDashboard() {
  const [pendingDrafts, setPendingDrafts] = useState<Draft[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getDrafts('pending')
      .then(setPendingDrafts)
      .catch(e => setError(e instanceof Error ? e.message : 'Ошибка'))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="p-8">
      <h1 className="text-xl font-bold text-zinc-900 mb-6">Операторский дашборд</h1>

      {error && (
        <div className="mb-4 px-4 py-3 rounded-lg bg-red-50 border border-red-200 text-sm text-red-700">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-4 mb-8">
        <div className="bg-white border border-zinc-200 rounded-xl p-5 shadow-sm">
          <p className="text-sm text-zinc-500 mb-1">Черновики на ревью</p>
          <p className="text-3xl font-bold text-orange-600">
            {loading ? '…' : pendingDrafts.length}
          </p>
        </div>
        <div className="bg-white border border-zinc-200 rounded-xl p-5 shadow-sm">
          <p className="text-sm text-zinc-500 mb-2">Словари логинов</p>
          <a href="/login-words" className="text-orange-500 hover:underline text-sm">
            Управление →
          </a>
        </div>
        <div className="bg-white border border-zinc-200 rounded-xl p-5 shadow-sm">
          <p className="text-sm text-zinc-500 mb-2">Загрузка материалов</p>
          <a href="/upload" className="text-orange-500 hover:underline text-sm">
            Загрузить →
          </a>
        </div>
      </div>

      {pendingDrafts.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold text-zinc-700 mb-3">Последние черновики на ревью</h2>
          <div className="bg-white border border-zinc-200 rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-zinc-50 border-b border-zinc-200">
                <tr>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">ID</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Тип</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Тема</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Дата</th>
                </tr>
              </thead>
              <tbody>
                {pendingDrafts.slice(0, 5).map(d => (
                  <tr key={d.id} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                    <td className="px-4 py-2.5 font-mono text-xs text-zinc-500">{d.id.slice(0, 8)}…</td>
                    <td className="px-4 py-2.5">{d.type}</td>
                    <td className="px-4 py-2.5">{d.topic ?? '—'}</td>
                    <td className="px-4 py-2.5 text-zinc-500">{new Date(d.created_at).toLocaleDateString('ru')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
