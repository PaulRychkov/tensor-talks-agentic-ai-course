import { useState } from 'react'

export interface InterviewParams {
  level: 'junior' | 'middle' | 'senior'
  topic: 'classic_ml' | 'nlp' | 'llm'
  type: 'interview' | 'training'
}

interface InterviewParamsModalProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: (params: InterviewParams) => void
}

export default function InterviewParamsModal({
  isOpen,
  onClose,
  onConfirm,
}: InterviewParamsModalProps) {
  const [level, setLevel] = useState<'junior' | 'middle' | 'senior'>('middle')
  const [topic, setTopic] = useState<'classic_ml' | 'nlp' | 'llm'>('classic_ml')
  const [type, setType] = useState<'interview' | 'training'>('interview')

  if (!isOpen) return null

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onConfirm({ level, topic, type })
  }

  const topicLabels: Record<string, string> = {
    classic_ml: 'Classic ML',
    nlp: 'NLP',
    llm: 'LLM',
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 max-w-md w-full mx-4">
        <h2 className="text-xl font-semibold mb-4">Параметры интервью</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-zinc-700 mb-2">
              Уровень
            </label>
            <div className="flex gap-2">
              {(['junior', 'middle', 'senior'] as const).map((l) => (
                <button
                  key={l}
                  type="button"
                  onClick={() => setLevel(l)}
                  className={`flex-1 px-4 py-2 rounded-lg border ${
                    level === l
                      ? 'bg-orange-600 text-white border-orange-600'
                      : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
                  }`}
                >
                  {l === 'junior' ? 'Junior' : l === 'middle' ? 'Middle' : 'Senior'}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-zinc-700 mb-2">
              Тема
            </label>
            <div className="flex gap-2">
              {(['classic_ml', 'nlp', 'llm'] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setTopic(t)}
                  className={`flex-1 px-4 py-2 rounded-lg border ${
                    topic === t
                      ? 'bg-orange-600 text-white border-orange-600'
                      : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
                  }`}
                >
                  {topicLabels[t]}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-zinc-700 mb-2">
              Тип
            </label>
            <div className="flex gap-2">
              {(['interview', 'training'] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setType(t)}
                  className={`flex-1 px-4 py-2 rounded-lg border ${
                    type === t
                      ? 'bg-orange-600 text-white border-orange-600'
                      : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
                  }`}
                >
                  {t === 'interview' ? 'Интервью' : 'Тренировка'}
                </button>
              ))}
            </div>
          </div>

          <div className="flex gap-2 pt-4">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 px-4 py-2 rounded-lg border border-zinc-300 text-zinc-700 hover:bg-zinc-50"
            >
              Отмена
            </button>
            <button
              type="submit"
              className="flex-1 px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700"
            >
              Начать
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

