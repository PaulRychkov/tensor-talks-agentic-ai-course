import { useState, useEffect } from 'react'
import { searchQuestions, createQuestion, updateQuestion, deleteQuestion, type Question, type QuestionPayload } from '../services/api'

const TOPICS = ['', 'classic_ml', 'nlp', 'llm', 'cv', 'data_science']
const TOPIC_LABELS: Record<string, string> = {
  '': 'Все темы', classic_ml: 'Classic ML', nlp: 'NLP', llm: 'LLM', cv: 'CV', data_science: 'DS',
}
const QTYPES = ['conceptual', 'practical', 'coding', 'case', 'applied']
const COMPLEXITY: Record<number, { label: string; cls: string }> = {
  1: { label: 'Easy', cls: 'bg-green-50 text-green-700' },
  2: { label: 'Medium', cls: 'bg-yellow-50 text-yellow-700' },
  3: { label: 'Hard', cls: 'bg-red-50 text-red-700' },
}

interface FormState {
  topic: string
  question_type: string
  complexity: number
  question_text: string
  ideal_answer: string
  expected_points: string
}

const emptyForm = (): FormState => ({
  topic: 'classic_ml',
  question_type: 'conceptual',
  complexity: 2,
  question_text: '',
  ideal_answer: '',
  expected_points: '',
})

function formFromQuestion(q: Question): FormState {
  return {
    topic: q.topic,
    question_type: q.question_type,
    complexity: q.complexity,
    question_text: q.question_text,
    ideal_answer: q.ideal_answer ?? '',
    expected_points: (q.expected_points ?? []).join('\n'),
  }
}

function formToPayload(form: FormState, existingId?: string): QuestionPayload {
  const points = form.expected_points
    .split('\n')
    .map(s => s.trim())
    .filter(Boolean)
  return {
    id: existingId,
    question_type: form.question_type,
    complexity: form.complexity,
    content: { question: form.question_text, expected_points: points },
    ideal_answer: { text: form.ideal_answer },
    metadata: { topic: form.topic, language: 'ru', created_by: 'admin' },
  }
}

export default function Questions() {
  const [query, setQuery] = useState('')
  const [topic, setTopic] = useState('')
  const [items, setItems] = useState<Question[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)

  // Modal state
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Question | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm())
  const [saving, setSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)

  const load = async (q: string, t: string) => {
    setLoading(true)
    setError(null)
    try {
      setItems(await searchQuestions(q || undefined, t || undefined))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load('', '') }, [])

  const handleSearch = (e: React.FormEvent) => { e.preventDefault(); load(query, topic) }

  const openCreate = () => {
    setEditTarget(null)
    setForm(emptyForm())
    setModalError(null)
    setModalOpen(true)
  }

  const openEdit = (q: Question) => {
    setEditTarget(q)
    setForm(formFromQuestion(q))
    setModalError(null)
    setModalOpen(true)
  }

  const handleDelete = async (q: Question) => {
    if (!confirm(`Удалить вопрос «${q.question_text.substring(0, 60)}…»?`)) return
    try {
      await deleteQuestion(q.id)
      setItems(prev => prev.filter(i => i.id !== q.id))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Ошибка удаления')
    }
  }

  const handleSave = async () => {
    if (!form.question_text.trim()) { setModalError('Введите текст вопроса'); return }
    setSaving(true)
    setModalError(null)
    try {
      if (editTarget) {
        await updateQuestion(editTarget.id, formToPayload(form, editTarget.id))
      } else {
        await createQuestion(formToPayload(form))
      }
      setModalOpen(false)
      await load(query, topic)
    } catch (e) {
      setModalError(e instanceof Error ? e.message : 'Ошибка сохранения')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-5">
        <h1 className="text-xl font-bold text-zinc-900">Вопросы</h1>
        <button
          onClick={openCreate}
          className="px-4 py-2 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 transition-colors"
        >
          + Добавить вопрос
        </button>
      </div>

      <form onSubmit={handleSearch} className="flex gap-3 mb-5">
        <input
          type="text"
          value={query}
          onChange={e => setQuery(e.target.value)}
          placeholder="Поиск по тексту…"
          className="flex-1 px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
        />
        <select
          value={topic}
          onChange={e => setTopic(e.target.value)}
          className="px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
        >
          {TOPICS.map(t => <option key={t} value={t}>{TOPIC_LABELS[t] ?? t}</option>)}
        </select>
        <button
          type="submit"
          className="px-4 py-2 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 transition-colors"
        >
          Найти
        </button>
      </form>

      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {loading && <p className="text-sm text-zinc-500">Загрузка…</p>}

      {!loading && (
        <>
          <div className="bg-white border border-zinc-200 rounded-xl overflow-hidden">
            {items.length === 0 ? (
              <p className="text-sm text-zinc-500 px-5 py-8 text-center">Ничего не найдено</p>
            ) : (
              <div className="overflow-y-auto" style={{ maxHeight: '560px' }}>
                <table className="w-full text-sm">
                  <thead className="bg-zinc-50 border-b border-zinc-200">
                    <tr>
                      <th className="text-left px-4 py-2.5 text-zinc-600 font-medium w-24">Сложность</th>
                      <th className="text-left px-4 py-2.5 text-zinc-600 font-medium w-32">Тема</th>
                      <th className="text-left px-4 py-2.5 text-zinc-600 font-medium">Вопрос</th>
                      <th className="px-4 py-2.5 w-24"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map(q => (
                      <>
                        <tr
                          key={q.id}
                          className="border-b border-zinc-100 hover:bg-zinc-50 cursor-pointer"
                          onClick={() => setExpanded(expanded === q.id ? null : q.id)}
                        >
                          <td className="px-4 py-2.5">
                            <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${COMPLEXITY[q.complexity]?.cls ?? 'bg-zinc-100 text-zinc-500'}`}>
                              {COMPLEXITY[q.complexity]?.label ?? q.complexity}
                            </span>
                          </td>
                          <td className="px-4 py-2.5">
                            <span className="text-xs text-zinc-500 truncate block max-w-[120px]" title={q.topic}>{q.topic}</span>
                          </td>
                          <td className="px-4 py-2.5 text-zinc-800">{q.question_text}</td>
                          <td className="px-4 py-2.5" onClick={e => e.stopPropagation()}>
                            <div className="flex gap-2 justify-end">
                              <button
                                onClick={() => openEdit(q)}
                                className="text-xs px-2 py-1 rounded bg-zinc-100 hover:bg-zinc-200 text-zinc-600 transition-colors"
                              >
                                Ред.
                              </button>
                              <button
                                onClick={() => handleDelete(q)}
                                className="text-xs px-2 py-1 rounded bg-red-50 hover:bg-red-100 text-red-600 transition-colors"
                              >
                                Удал.
                              </button>
                            </div>
                          </td>
                        </tr>
                        {expanded === q.id && (
                          <tr key={`${q.id}-exp`} className="border-b border-zinc-100 bg-orange-50">
                            <td colSpan={4} className="px-4 py-3">
                              {q.ideal_answer && (
                                <div className="mb-2">
                                  <span className="text-xs font-semibold text-orange-700 uppercase tracking-wide">Ответ</span>
                                  <p className="text-sm text-zinc-700 mt-1">{q.ideal_answer}</p>
                                </div>
                              )}
                              {q.expected_points && q.expected_points.length > 0 && (
                                <div>
                                  <span className="text-xs font-semibold text-orange-700 uppercase tracking-wide">Ключевые пункты</span>
                                  <ul className="mt-1 list-disc list-inside text-sm text-zinc-600 space-y-0.5">
                                    {q.expected_points.map((p, i) => <li key={i}>{p}</li>)}
                                  </ul>
                                </div>
                              )}
                            </td>
                          </tr>
                        )}
                      </>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
          <p className="text-xs text-zinc-400 mt-3">{items.length} вопросов</p>
        </>
      )}

      {/* Create/Edit modal */}
      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setModalOpen(false)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-semibold mb-4 text-zinc-900">
              {editTarget ? 'Редактировать вопрос' : 'Новый вопрос'}
            </h2>

            <div className="space-y-3">
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium text-zinc-600 mb-1">Тема</label>
                  <select
                    value={form.topic}
                    onChange={e => setForm(f => ({ ...f, topic: e.target.value }))}
                    className="w-full px-2 py-1.5 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
                  >
                    {TOPICS.filter(Boolean).map(t => <option key={t} value={t}>{TOPIC_LABELS[t] ?? t}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-zinc-600 mb-1">Тип</label>
                  <select
                    value={form.question_type}
                    onChange={e => setForm(f => ({ ...f, question_type: e.target.value }))}
                    className="w-full px-2 py-1.5 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
                  >
                    {QTYPES.map(t => <option key={t} value={t}>{t}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-zinc-600 mb-1">Сложность</label>
                  <select
                    value={form.complexity}
                    onChange={e => setForm(f => ({ ...f, complexity: Number(e.target.value) }))}
                    className="w-full px-2 py-1.5 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
                  >
                    <option value={1}>1 — Easy</option>
                    <option value={2}>2 — Medium</option>
                    <option value={3}>3 — Hard</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-zinc-600 mb-1">Текст вопроса</label>
                <textarea
                  rows={3}
                  value={form.question_text}
                  onChange={e => setForm(f => ({ ...f, question_text: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400 resize-none"
                  placeholder="Сформулируйте вопрос…"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-zinc-600 mb-1">Идеальный ответ</label>
                <textarea
                  rows={3}
                  value={form.ideal_answer}
                  onChange={e => setForm(f => ({ ...f, ideal_answer: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400 resize-none"
                  placeholder="Образцовый ответ…"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-zinc-600 mb-1">Ключевые пункты (по одному на строке)</label>
                <textarea
                  rows={3}
                  value={form.expected_points}
                  onChange={e => setForm(f => ({ ...f, expected_points: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400 resize-none"
                  placeholder="Пункт 1&#10;Пункт 2&#10;Пункт 3"
                />
              </div>
            </div>

            {modalError && <p className="text-sm text-red-600 mt-3">{modalError}</p>}

            <div className="flex justify-end gap-3 mt-5">
              <button
                onClick={() => setModalOpen(false)}
                className="px-4 py-2 rounded-lg border border-zinc-300 text-sm text-zinc-700 hover:bg-zinc-50"
              >
                Отмена
              </button>
              <button
                onClick={handleSave}
                disabled={saving}
                className="px-4 py-2 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 disabled:opacity-50"
              >
                {saving ? 'Сохранение…' : (editTarget ? 'Сохранить' : 'Создать')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
