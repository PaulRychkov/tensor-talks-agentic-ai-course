import { useState, useEffect } from 'react'
import { searchKnowledge, updateKnowledgeItem, deleteKnowledgeItem, type KnowledgeItem, type KnowledgePayload } from '../services/api'

const TOPICS = ['', 'classic_ml', 'nlp', 'llm']
const TOPIC_LABELS: Record<string, string> = { '': 'Все темы', classic_ml: 'Classic ML', nlp: 'NLP', llm: 'LLM' }
const COMPLEXITY_LABEL: Record<number, string> = { 1: '★', 2: '★★', 3: '★★★' }

interface EditForm {
  concept: string
  complexity: number
  tags: string
  segments: { type: string; content: string }[]
}

function itemToForm(item: KnowledgeItem): EditForm {
  return {
    concept: item.concept,
    complexity: item.complexity,
    tags: item.tags.join(', '),
    segments: item.segments?.length ? item.segments : [{ type: 'definition', content: item.summary }],
  }
}

export default function Knowledge() {
  const [query, setQuery] = useState('')
  const [topic, setTopic] = useState('')
  const [items, setItems] = useState<KnowledgeItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)

  // Edit modal
  const [editItem, setEditItem] = useState<KnowledgeItem | null>(null)
  const [editForm, setEditForm] = useState<EditForm | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const load = async (q: string, t: string) => {
    setLoading(true)
    setError(null)
    try {
      setItems(await searchKnowledge(q || undefined, t || undefined))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load('', '') }, [])

  const handleSearch = (e: React.FormEvent) => { e.preventDefault(); load(query, topic) }

  const openEdit = (item: KnowledgeItem, e: React.MouseEvent) => {
    e.stopPropagation()
    setEditItem(item)
    setEditForm(itemToForm(item))
    setSaveError(null)
  }

  const handleSave = async () => {
    if (!editItem || !editForm) return
    setSaving(true)
    setSaveError(null)
    try {
      const payload: KnowledgePayload = {
        concept: editForm.concept,
        complexity: editForm.complexity,
        tags: editForm.tags.split(',').map(t => t.trim()).filter(Boolean),
        data: { segments: editForm.segments },
      }
      await updateKnowledgeItem(editItem.id, payload)
      setEditItem(null)
      setEditForm(null)
      load(query, topic)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Ошибка сохранения')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (item: KnowledgeItem, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!confirm(`Удалить «${item.concept}»?`)) return
    try {
      await deleteKnowledgeItem(item.id)
      load(query, topic)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Ошибка удаления')
    }
  }

  const updateSegment = (idx: number, field: 'type' | 'content', value: string) => {
    if (!editForm) return
    const segs = [...editForm.segments]
    segs[idx] = { ...segs[idx], [field]: value }
    setEditForm({ ...editForm, segments: segs })
  }

  const addSegment = () => {
    if (!editForm) return
    setEditForm({ ...editForm, segments: [...editForm.segments, { type: 'example', content: '' }] })
  }

  const removeSegment = (idx: number) => {
    if (!editForm) return
    setEditForm({ ...editForm, segments: editForm.segments.filter((_, i) => i !== idx) })
  }

  return (
    <div className="p-8">
      <h1 className="text-xl font-bold text-zinc-900 mb-5">База знаний</h1>

      <form onSubmit={handleSearch} className="flex gap-3 mb-5">
        <input
          type="text"
          value={query}
          onChange={e => setQuery(e.target.value)}
          placeholder="Поиск по концепции…"
          className="flex-1 px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
        />
        <select
          value={topic}
          onChange={e => setTopic(e.target.value)}
          className="px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
        >
          {TOPICS.map(t => <option key={t} value={t}>{TOPIC_LABELS[t]}</option>)}
        </select>
        <button type="submit" className="px-4 py-2 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 transition-colors">
          Найти
        </button>
      </form>

      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {loading && <p className="text-sm text-zinc-500">Загрузка…</p>}

      {!loading && (
        <>
          <div className="overflow-y-auto space-y-1.5" style={{ maxHeight: '560px' }}>
            {items.length === 0 ? (
              <p className="text-sm text-zinc-500 py-8 text-center">Ничего не найдено</p>
            ) : items.map(item => (
              <div key={item.id} className="bg-white border border-zinc-200 rounded-xl overflow-hidden">
                <button
                  type="button"
                  className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-zinc-50 transition-colors"
                  onClick={() => setExpanded(expanded === item.id ? null : item.id)}
                >
                  <span className="text-yellow-500 text-xs w-8 flex-shrink-0">{COMPLEXITY_LABEL[item.complexity] ?? ''}</span>
                  <span className="flex-1 text-sm font-medium text-zinc-800 truncate">{item.concept}</span>
                  {item.tags.length > 0 && (
                    <div className="hidden sm:flex gap-1 flex-shrink-0">
                      {item.tags.slice(0, 2).map(tag => (
                        <span key={tag} className="px-1.5 py-0.5 bg-zinc-100 text-zinc-500 text-xs rounded">{tag}</span>
                      ))}
                    </div>
                  )}
                  <span
                    className="px-2 py-0.5 rounded text-xs bg-blue-50 text-blue-600 hover:bg-blue-100 flex-shrink-0"
                    onClick={e => openEdit(item, e)}
                    role="button"
                  >
                    Ред.
                  </span>
                  <span
                    className="px-2 py-0.5 rounded text-xs bg-red-50 text-red-600 hover:bg-red-100 flex-shrink-0"
                    onClick={e => handleDelete(item, e)}
                    role="button"
                  >
                    Удал.
                  </span>
                  <span className="text-zinc-300 text-xs flex-shrink-0">{expanded === item.id ? '▲' : '▼'}</span>
                </button>

                {expanded === item.id && (
                  <div className="border-t border-zinc-100">
                    {item.summary && (
                      <div className="px-4 py-3 bg-blue-50 border-b border-zinc-100">
                        <p className="text-xs font-semibold text-blue-700 uppercase tracking-wide mb-1">Определение</p>
                        <p className="text-sm text-zinc-700">{item.summary}</p>
                      </div>
                    )}
                    {item.segments && item.segments.filter(s => s.type !== 'definition').map((seg, i) => (
                      <div key={i} className="px-4 py-2.5 border-b border-zinc-100 last:border-0">
                        <p className="text-xs font-semibold text-zinc-500 uppercase tracking-wide mb-1">{seg.type}</p>
                        <p className="text-sm text-zinc-700 whitespace-pre-wrap">{seg.content}</p>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
          <p className="text-xs text-zinc-400 mt-3">{items.length} записей</p>
        </>
      )}

      {/* Edit Modal */}
      {editItem && editForm && (
        <div className="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-2xl shadow-xl w-full max-w-2xl max-h-[90vh] flex flex-col">
            <div className="px-6 py-4 border-b border-zinc-100 flex items-center justify-between">
              <h2 className="font-semibold text-zinc-900">Редактировать статью</h2>
              <button onClick={() => setEditItem(null)} className="text-zinc-400 hover:text-zinc-700 text-xl">✕</button>
            </div>
            <div className="overflow-y-auto p-6 space-y-4 flex-1">
              <div>
                <label className="block text-xs font-medium text-zinc-600 mb-1">Концепция</label>
                <input
                  className="w-full px-3 py-2 text-sm border border-zinc-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-orange-400"
                  value={editForm.concept}
                  onChange={e => setEditForm({ ...editForm, concept: e.target.value })}
                />
              </div>
              <div className="flex gap-4">
                <div className="flex-1">
                  <label className="block text-xs font-medium text-zinc-600 mb-1">Сложность</label>
                  <select
                    className="w-full px-3 py-2 text-sm border border-zinc-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-orange-400"
                    value={editForm.complexity}
                    onChange={e => setEditForm({ ...editForm, complexity: Number(e.target.value) })}
                  >
                    <option value={1}>★ Базовый</option>
                    <option value={2}>★★ Средний</option>
                    <option value={3}>★★★ Продвинутый</option>
                  </select>
                </div>
                <div className="flex-1">
                  <label className="block text-xs font-medium text-zinc-600 mb-1">Теги (через запятую)</label>
                  <input
                    className="w-full px-3 py-2 text-sm border border-zinc-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-orange-400"
                    value={editForm.tags}
                    onChange={e => setEditForm({ ...editForm, tags: e.target.value })}
                    placeholder="ml, supervised, regression"
                  />
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-2">
                  <label className="text-xs font-medium text-zinc-600">Сегменты контента</label>
                  <button onClick={addSegment} className="text-xs px-2 py-1 rounded bg-orange-50 text-orange-600 hover:bg-orange-100">+ Добавить</button>
                </div>
                <div className="space-y-3">
                  {editForm.segments.map((seg, idx) => (
                    <div key={idx} className="border border-zinc-200 rounded-lg p-3 space-y-2">
                      <div className="flex items-center gap-2">
                        <select
                          className="px-2 py-1 text-xs border border-zinc-200 rounded focus:outline-none focus:ring-1 focus:ring-orange-400"
                          value={seg.type}
                          onChange={e => updateSegment(idx, 'type', e.target.value)}
                        >
                          {['definition', 'example', 'formula', 'code', 'note', 'interview_hint'].map(t => (
                            <option key={t} value={t}>{t}</option>
                          ))}
                        </select>
                        <button onClick={() => removeSegment(idx)} className="ml-auto text-xs text-red-400 hover:text-red-600">✕</button>
                      </div>
                      <textarea
                        className="w-full px-2 py-1.5 text-sm border border-zinc-200 rounded focus:outline-none focus:ring-1 focus:ring-orange-400 resize-y"
                        rows={3}
                        value={seg.content}
                        onChange={e => updateSegment(idx, 'content', e.target.value)}
                      />
                    </div>
                  ))}
                </div>
              </div>

              {saveError && <p className="text-sm text-red-600">{saveError}</p>}
            </div>
            <div className="px-6 py-4 border-t border-zinc-100 flex justify-end gap-3">
              <button onClick={() => setEditItem(null)} className="px-4 py-2 text-sm rounded-lg border border-zinc-200 text-zinc-600 hover:bg-zinc-50">
                Отмена
              </button>
              <button
                onClick={handleSave}
                disabled={saving}
                className="px-4 py-2 text-sm rounded-lg bg-orange-600 text-white hover:bg-orange-700 disabled:opacity-50"
              >
                {saving ? 'Сохранение…' : 'Сохранить'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
