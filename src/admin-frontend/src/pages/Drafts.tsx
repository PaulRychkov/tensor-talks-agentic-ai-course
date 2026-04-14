import { useState, useEffect, useCallback } from 'react'
import { getDrafts, getDraft, approveDraft, rejectDraft, type Draft } from '../services/api'

type StatusFilter = 'pending' | 'approved' | 'rejected' | ''

export default function Drafts() {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('pending')
  const [drafts, setDrafts] = useState<Draft[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Draft | null>(null)
  const [rejectComment, setRejectComment] = useState('')
  const [actionLoading, setActionLoading] = useState(false)

  const load = useCallback(async (status: StatusFilter) => {
    setLoading(true)
    setError(null)
    try {
      setDrafts(await getDrafts(status || undefined))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load(statusFilter) }, [statusFilter, load])

  const openDraft = async (id: string) => {
    try {
      const d = await getDraft(id)
      setSelected(d)
      setRejectComment('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка загрузки черновика')
    }
  }

  const handleApprove = async () => {
    if (!selected) return
    setActionLoading(true)
    try {
      await approveDraft(selected.id)
      setSelected(null)
      await load(statusFilter)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setActionLoading(false)
    }
  }

  const handleReject = async () => {
    if (!selected) return
    setActionLoading(true)
    try {
      await rejectDraft(selected.id, rejectComment || undefined)
      setSelected(null)
      await load(statusFilter)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setActionLoading(false)
    }
  }

  const STATUS_LABELS: Record<string, string> = {
    pending: 'На ревью', approved: 'Одобрено', rejected: 'Отклонено',
  }

  return (
    <div className="p-8">
      <h1 className="text-xl font-bold text-zinc-900 mb-5">Очередь черновиков</h1>

      <div className="flex gap-2 mb-5">
        {(['pending', 'approved', 'rejected', ''] as StatusFilter[]).map(s => (
          <button
            key={s}
            onClick={() => setStatusFilter(s)}
            className={`px-3 py-1.5 rounded-lg border text-sm transition-colors ${
              statusFilter === s
                ? 'bg-orange-600 text-white border-orange-600'
                : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
            }`}
          >
            {s === '' ? 'Все' : STATUS_LABELS[s]}
          </button>
        ))}
      </div>

      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {loading && <p className="text-sm text-zinc-500">Загрузка…</p>}

      {!loading && (
        <div className="bg-white border border-zinc-200 rounded-xl overflow-hidden">
          {drafts.length === 0 ? (
            <p className="text-sm text-zinc-500 px-5 py-8 text-center">Черновиков нет</p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-zinc-50 border-b border-zinc-200">
                <tr>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">ID</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Тип</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Тема</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Статус</th>
                  <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Дата</th>
                  <th className="px-4 py-2.5" />
                </tr>
              </thead>
              <tbody>
                {drafts.map(d => (
                  <tr key={d.id} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                    <td className="px-4 py-2.5 font-mono text-xs text-zinc-500">{d.id.slice(0, 8)}…</td>
                    <td className="px-4 py-2.5">{d.type}</td>
                    <td className="px-4 py-2.5">{d.topic ?? '—'}</td>
                    <td className="px-4 py-2.5">
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                        d.status === 'pending' ? 'bg-yellow-50 text-yellow-700' :
                        d.status === 'approved' ? 'bg-green-50 text-green-700' :
                        'bg-red-50 text-red-700'
                      }`}>
                        {STATUS_LABELS[d.status] ?? d.status}
                      </span>
                      {d.duplicate_candidate && (
                        <span className="ml-1.5 px-1.5 py-0.5 rounded text-xs bg-orange-50 text-orange-600">дубль?</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-zinc-500">{new Date(d.created_at).toLocaleDateString('ru')}</td>
                    <td className="px-4 py-2.5">
                      <button
                        onClick={() => openDraft(d.id)}
                        className="text-orange-600 hover:underline text-xs"
                      >
                        Просмотр
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {/* Preview modal */}
      {selected && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-2xl shadow-xl max-w-2xl w-full max-h-[85vh] flex flex-col">
            <div className="px-6 py-4 border-b border-zinc-100 flex items-center justify-between">
              <div>
                <h2 className="font-semibold text-zinc-900">{selected.title ?? 'Черновик'}</h2>
                <p className="text-xs text-zinc-500">{selected.type} · {selected.topic}</p>
              </div>
              <button onClick={() => setSelected(null)} className="text-zinc-400 hover:text-zinc-600 text-xl leading-none">×</button>
            </div>
            <div className="flex-1 overflow-auto px-6 py-4">
              {selected.content ? (
                <pre className="text-sm text-zinc-700 whitespace-pre-wrap font-sans">{selected.content}</pre>
              ) : (
                <p className="text-sm text-zinc-400">Содержимое не загружено</p>
              )}
            </div>
            {selected.status === 'pending' && (
              <div className="px-6 py-4 border-t border-zinc-100 space-y-3">
                <textarea
                  value={rejectComment}
                  onChange={e => setRejectComment(e.target.value)}
                  placeholder="Комментарий при отклонении (необязательно)"
                  rows={2}
                  className="w-full px-3 py-2 text-sm rounded-lg border border-zinc-300 resize-none focus:outline-none focus:ring-2 focus:ring-orange-400"
                />
                <div className="flex gap-3">
                  <button
                    onClick={handleReject}
                    disabled={actionLoading}
                    className="flex-1 py-2 rounded-lg border border-red-300 text-red-700 text-sm hover:bg-red-50 disabled:opacity-50 transition-colors"
                  >
                    Отклонить
                  </button>
                  <button
                    onClick={handleApprove}
                    disabled={actionLoading}
                    className="flex-1 py-2 rounded-lg bg-green-600 text-white text-sm hover:bg-green-700 disabled:opacity-50 transition-colors"
                  >
                    Одобрить
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
