import { useState, useEffect, useCallback } from 'react'
import {
  getAdjectives, createAdjective, updateAdjective, deleteAdjective,
  getNouns, createNoun, updateNoun, deleteNoun,
  type LoginWord,
} from '../services/api'

function WordPanel({
  title,
  words,
  loading,
  onCreate,
  onUpdate,
  onDelete,
}: {
  title: string
  words: LoginWord[]
  loading: boolean
  onCreate: (word: string) => Promise<void>
  onUpdate: (id: number, word: string) => Promise<void>
  onDelete: (id: number) => Promise<void>
}) {
  const [newWord, setNewWord] = useState('')
  const [editId, setEditId] = useState<number | null>(null)
  const [editWord, setEditWord] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newWord.trim()) return
    setSubmitting(true)
    setError(null)
    try { await onCreate(newWord.trim()); setNewWord('') }
    catch (err) { setError(err instanceof Error ? err.message : 'Ошибка') }
    finally { setSubmitting(false) }
  }

  const startEdit = (w: LoginWord) => {
    setEditId(w.id)
    setEditWord(w.word)
    setError(null)
  }

  const handleSave = async (id: number) => {
    if (!editWord.trim()) return
    setSubmitting(true)
    setError(null)
    try { await onUpdate(id, editWord.trim()); setEditId(null) }
    catch (err) { setError(err instanceof Error ? err.message : 'Ошибка') }
    finally { setSubmitting(false) }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Удалить слово?')) return
    setSubmitting(true)
    setError(null)
    try { await onDelete(id) }
    catch (err) { setError(err instanceof Error ? err.message : 'Ошибка') }
    finally { setSubmitting(false) }
  }

  return (
    <div className="flex flex-col bg-white border border-zinc-200 rounded-xl overflow-hidden">
      <div className="px-4 py-3 border-b border-zinc-100 bg-zinc-50 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-zinc-800">{title}</h2>
        <span className="text-xs text-zinc-400">{words.length} слов</span>
      </div>

      {/* Add form */}
      <form onSubmit={handleCreate} className="px-4 py-3 border-b border-zinc-100 flex gap-2">
        <input
          type="text"
          value={newWord}
          onChange={e => setNewWord(e.target.value)}
          placeholder="Новое слово (CamelCase)…"
          className="flex-1 px-3 py-1.5 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400 focus:border-orange-400"
        />
        <button
          type="submit"
          disabled={submitting || !newWord.trim()}
          className="px-3 py-1.5 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 disabled:opacity-50 transition-colors whitespace-nowrap"
        >
          + Добавить
        </button>
      </form>

      {error && (
        <div className="px-4 py-2 text-xs text-red-600 bg-red-50 border-b border-red-100">{error}</div>
      )}

      {/* Scrollable word list */}
      <div className="overflow-y-auto flex-1" style={{ maxHeight: '420px' }}>
        {loading ? (
          <p className="text-sm text-zinc-400 px-4 py-6 text-center">Загрузка…</p>
        ) : words.length === 0 ? (
          <p className="text-sm text-zinc-400 px-4 py-6 text-center">Список пуст</p>
        ) : (
          <table className="w-full text-sm">
            <tbody>
              {words.map(w => (
                <tr key={w.id} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                  <td className="pl-4 pr-2 py-2 text-zinc-300 font-mono text-xs w-10">{w.id}</td>
                  <td className="px-2 py-2">
                    {editId === w.id ? (
                      <input
                        type="text"
                        value={editWord}
                        onChange={e => setEditWord(e.target.value)}
                        onKeyDown={e => { if (e.key === 'Enter') handleSave(w.id); if (e.key === 'Escape') setEditId(null) }}
                        className="px-2 py-1 text-sm rounded border border-orange-400 focus:outline-none focus:ring-1 focus:ring-orange-400 w-full"
                        autoFocus
                      />
                    ) : (
                      <span className="font-medium text-zinc-800">{w.word}</span>
                    )}
                  </td>
                  <td className="px-2 py-2 text-right pr-4 w-28">
                    <div className="flex gap-2 justify-end">
                      {editId === w.id ? (
                        <>
                          <button
                            onClick={() => handleSave(w.id)}
                            disabled={submitting}
                            className="text-xs text-green-600 hover:underline disabled:opacity-50"
                          >
                            ✓ Сохранить
                          </button>
                          <button onClick={() => setEditId(null)} className="text-xs text-zinc-400 hover:underline">
                            Отмена
                          </button>
                        </>
                      ) : (
                        <>
                          <button onClick={() => startEdit(w)} className="text-xs text-orange-600 hover:underline">
                            Изм.
                          </button>
                          <button
                            onClick={() => handleDelete(w.id)}
                            disabled={submitting}
                            className="text-xs text-red-400 hover:underline disabled:opacity-50"
                          >
                            ✕
                          </button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

export default function LoginWords() {
  const [adjectives, setAdjectives] = useState<LoginWord[]>([])
  const [nouns, setNouns] = useState<LoginWord[]>([])
  const [loading, setLoading] = useState(true)

  const loadAll = useCallback(async () => {
    setLoading(true)
    try {
      const [adjs, ns] = await Promise.all([getAdjectives(), getNouns()])
      setAdjectives(adjs)
      setNouns(ns)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadAll() }, [loadAll])

  return (
    <div className="p-8">
      <h1 className="text-xl font-bold text-zinc-900 mb-1">Словари логинов</h1>
      <p className="text-sm text-zinc-500 mb-5">Прилагательные и существительные для генерации логинов пользователей</p>

      <div className="grid grid-cols-2 gap-5">
        <WordPanel
          title="Прилагательные"
          words={adjectives}
          loading={loading}
          onCreate={async (word) => { await createAdjective(word); await loadAll() }}
          onUpdate={async (id, word) => { await updateAdjective(id, word); await loadAll() }}
          onDelete={async (id) => { await deleteAdjective(id); await loadAll() }}
        />
        <WordPanel
          title="Существительные"
          words={nouns}
          loading={loading}
          onCreate={async (word) => { await createNoun(word); await loadAll() }}
          onUpdate={async (id, word) => { await updateNoun(id, word); await loadAll() }}
          onDelete={async (id) => { await deleteNoun(id); await loadAll() }}
        />
      </div>
    </div>
  )
}
