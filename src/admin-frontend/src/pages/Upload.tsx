import { useState } from 'react'
import { uploadUrl, uploadFile, type UploadResult } from '../services/api'

const TOPICS = ['classic_ml', 'nlp', 'llm']

type UploadMode = 'url' | 'file'

export default function Upload() {
  const [mode, setMode] = useState<UploadMode>('url')
  const [url, setUrl] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [topic, setTopic] = useState('nlp')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<UploadResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      let res: UploadResult
      if (mode === 'url') {
        if (!url.trim()) return
        res = await uploadUrl(url.trim(), topic)
        setUrl('')
      } else {
        if (!file) return
        res = await uploadFile(file, topic)
        setFile(null)
      }
      setResult(res)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Ошибка загрузки')
    } finally {
      setLoading(false)
    }
  }

  const isValid = mode === 'url' ? url.trim().length > 0 : file !== null

  return (
    <div className="p-8 max-w-xl">
      <h1 className="text-xl font-bold text-zinc-900 mb-2">Загрузка материалов</h1>
      <p className="text-sm text-zinc-500 mb-6">
        Материал поступает в knowledge-producer-service, который создаёт черновик для ревью.
      </p>

      {/* Mode toggle */}
      <div className="flex gap-1 mb-5 bg-zinc-100 p-1 rounded-lg w-fit">
        <button
          type="button"
          onClick={() => setMode('url')}
          className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${
            mode === 'url' ? 'bg-white shadow-sm text-zinc-900' : 'text-zinc-500 hover:text-zinc-700'
          }`}
        >
          По URL
        </button>
        <button
          type="button"
          onClick={() => setMode('file')}
          className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${
            mode === 'file' ? 'bg-white shadow-sm text-zinc-900' : 'text-zinc-500 hover:text-zinc-700'
          }`}
        >
          Файл
        </button>
      </div>

      <div className="bg-white border border-zinc-200 rounded-xl p-6">
        <form onSubmit={handleSubmit} className="space-y-4">

          {mode === 'url' ? (
            <div>
              <label className="block text-sm font-medium text-zinc-700 mb-1.5">URL материала</label>
              <input
                type="url"
                value={url}
                onChange={e => setUrl(e.target.value)}
                placeholder="https://arxiv.org/abs/..."
                className="w-full px-3 py-2.5 text-sm rounded-lg border border-zinc-300 focus:outline-none focus:ring-2 focus:ring-orange-400"
                required
              />
            </div>
          ) : (
            <div>
              <label className="block text-sm font-medium text-zinc-700 mb-1.5">Файл</label>
              <input
                type="file"
                accept=".pdf,.md,.txt,.json"
                onChange={e => setFile(e.target.files?.[0] ?? null)}
                className="w-full text-sm text-zinc-600 file:mr-3 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-sm file:font-medium file:bg-orange-50 file:text-orange-700 hover:file:bg-orange-100"
              />
              <p className="text-xs text-zinc-400 mt-1">PDF, Markdown (.md), TXT, JSON</p>
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-zinc-700 mb-1.5">Тема</label>
            <div className="flex gap-2">
              {TOPICS.map(t => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setTopic(t)}
                  className={`px-4 py-2 rounded-lg border text-sm transition-colors ${
                    topic === t
                      ? 'bg-orange-600 text-white border-orange-600'
                      : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
                  }`}
                >
                  {t === 'classic_ml' ? 'Classic ML' : t.toUpperCase()}
                </button>
              ))}
            </div>
          </div>

          {error && (
            <div className="px-4 py-3 rounded-lg bg-red-50 border border-red-200 text-sm text-red-700">
              {error}
            </div>
          )}
          {result && (
            <div className="px-4 py-3 rounded-lg bg-green-50 border border-green-200 text-sm text-green-700">
              ✓ Черновик создан
              {result.draft_id && <span className="ml-2 text-xs text-green-600">draft: {result.draft_id}</span>}
              {result.task_id && <span className="ml-2 text-xs text-green-600">task: {result.task_id}</span>}
            </div>
          )}

          <button
            type="submit"
            disabled={loading || !isValid}
            className="w-full py-2.5 rounded-lg bg-orange-600 text-white text-sm font-medium hover:bg-orange-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Загрузка…' : 'Загрузить'}
          </button>
        </form>
      </div>

      <div className="mt-4 px-4 py-3 rounded-lg bg-zinc-50 border border-zinc-200 text-xs text-zinc-500">
        По URL: arXiv, Habr, официальная документация ML-библиотек.<br/>
        Файл: PDF (.pdf), Markdown (.md), текст (.txt), JSON (.json).<br/>
        После загрузки материал появится в очереди черновиков для проверки.
      </div>
    </div>
  )
}
