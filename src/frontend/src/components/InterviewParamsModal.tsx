import { useState, useEffect } from 'react'
import type { SessionMode } from '../services/chat'
import { getSubtopics, type SubtopicEntry } from '../services/dashboard'

export interface InterviewParams {
  level: 'junior' | 'middle' | 'senior'
  topic: 'classic_ml' | 'nlp' | 'llm'
  mode: SessionMode
  subtopics?: string[]
  num_questions?: number
}

interface InterviewParamsModalProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: (params: InterviewParams) => void
  defaultMode?: SessionMode
  defaultTopic?: 'classic_ml' | 'nlp' | 'llm'
  defaultSubtopics?: string[]
  title?: string
}

const MODE_DESCRIPTIONS: Record<SessionMode, string> = {
  interview: 'Полноценное ML-интервью с оценкой и отчётом',
  training: 'Практика по слабым темам с детальной обратной связью',
  study: 'Изучение теории + контрольные вопросы по теме',
}

export default function InterviewParamsModal({
  isOpen,
  onClose,
  onConfirm,
  defaultMode = 'interview',
  defaultTopic = 'classic_ml',
  defaultSubtopics,
  title = 'Параметры сессии',
}: InterviewParamsModalProps) {
  const [level, setLevel] = useState<'junior' | 'middle' | 'senior'>('middle')
  const [topic, setTopic] = useState<'classic_ml' | 'nlp' | 'llm'>(defaultTopic)
  const [mode, setMode] = useState<SessionMode>(defaultMode)
  const [selectedSubtopics, setSelectedSubtopics] = useState<string[]>(defaultSubtopics || [])
  const [duration, setDuration] = useState<'quick' | 'normal' | 'long'>('normal')

  // Dynamic subtopics from knowledge base
  const [allSubtopics, setAllSubtopics] = useState<SubtopicEntry[]>([])
  const [subtopicsLoading, setSubtopicsLoading] = useState(false)

  // Fetch subtopics on first open
  useEffect(() => {
    if (isOpen && allSubtopics.length === 0 && !subtopicsLoading) {
      setSubtopicsLoading(true)
      getSubtopics()
        .then(setAllSubtopics)
        .finally(() => setSubtopicsLoading(false))
    }
  }, [isOpen])

  useEffect(() => {
    if (isOpen) {
      setMode(defaultMode)
      setTopic(defaultTopic)
      setSelectedSubtopics(defaultSubtopics || [])
    }
  }, [isOpen, defaultMode, defaultTopic, defaultSubtopics])

  // Reset subtopics when topic changes
  useEffect(() => {
    if (!defaultSubtopics?.length) {
      setSelectedSubtopics([])
    }
  }, [topic])

  if (!isOpen) return null

  const toggleSubtopic = (id: string) => {
    setSelectedSubtopics(prev =>
      prev.includes(id) ? prev.filter(s => s !== id) : [...prev, id]
    )
  }

  const needsSubtopics = mode === 'training' || mode === 'study'
  // Filter subtopics for the currently selected topic
  const availableSubtopics = allSubtopics.filter(st => st.topics.includes(topic))
  const isValid = !needsSubtopics || selectedSubtopics.length > 0

  const DURATION_OPTIONS: { id: 'quick' | 'normal' | 'long'; label: string; questions: number }[] = [
    { id: 'quick', label: 'Быстрое', questions: 5 },
    { id: 'normal', label: 'Обычное', questions: 10 },
    { id: 'long', label: 'Длительное', questions: 15 },
  ]

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!isValid) return
    const selectedDuration = DURATION_OPTIONS.find(d => d.id === duration)
    onConfirm({
      level,
      topic,
      mode,
      subtopics: needsSubtopics && selectedSubtopics.length > 0 ? selectedSubtopics : undefined,
      num_questions: mode === 'interview' ? selectedDuration?.questions : undefined,
    })
  }

  const topicLabels: Record<string, string> = {
    classic_ml: 'Classic ML',
    nlp: 'NLP',
    llm: 'LLM',
  }

  const MODE_ICONS: Record<SessionMode, string> = {
    interview: '🎯',
    training: '💪',
    study: '📖',
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl p-6 max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
        {/* Заголовок — режим как заголовок, не кнопки */}
        <div className="mb-5">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-2xl">{MODE_ICONS[mode]}</span>
            <h2 className="text-xl font-bold text-zinc-900">{title}</h2>
          </div>
          <p className="text-sm text-zinc-500 pl-9">{MODE_DESCRIPTIONS[mode]}</p>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">

          <div>
            <label className="block text-sm font-medium text-zinc-700 mb-2">Уровень</label>
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
            <label className="block text-sm font-medium text-zinc-700 mb-2">Тема</label>
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

          {mode === 'interview' && (
            <div>
              <label className="block text-sm font-medium text-zinc-700 mb-2">Длительность</label>
              <div className="flex gap-2">
                {DURATION_OPTIONS.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    onClick={() => setDuration(opt.id)}
                    className={`flex-1 px-3 py-2 rounded-lg border text-sm transition-colors ${
                      duration === opt.id
                        ? 'bg-orange-600 text-white border-orange-600'
                        : 'bg-white text-zinc-700 border-zinc-300 hover:border-orange-300'
                    }`}
                  >
                    <span className="block font-medium">{opt.label}</span>
                    <span className="block text-xs opacity-75">{opt.questions} вопр.</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {needsSubtopics && (
            <div>
              <label className="block text-sm font-medium text-zinc-700 mb-2">
                Подтемы
                <span className="text-zinc-400 font-normal ml-1">(выберите хотя бы одну)</span>
              </label>
              {subtopicsLoading ? (
                <div className="text-sm text-zinc-400 py-2">Загрузка подтем...</div>
              ) : availableSubtopics.length === 0 ? (
                <div className="text-sm text-zinc-400 py-2">Нет доступных подтем для этой темы</div>
              ) : (
                <div className="flex flex-wrap gap-2 max-h-48 overflow-y-auto pr-1">
                  {availableSubtopics.map((st) => (
                    <button
                      key={st.id}
                      type="button"
                      onClick={() => toggleSubtopic(st.id)}
                      className={`px-3 py-1.5 rounded-lg border text-sm transition-colors ${
                        selectedSubtopics.includes(st.id)
                          ? 'bg-orange-600 text-white border-orange-600'
                          : 'bg-white text-zinc-700 border-zinc-200 hover:border-orange-300'
                      }`}
                    >
                      {st.label}
                    </button>
                  ))}
                </div>
              )}
              {!isValid && (
                <p className="text-xs text-red-500 mt-1">Выберите хотя бы одну подтему</p>
              )}
            </div>
          )}

          <div className="rounded-lg bg-amber-50 border border-amber-200 px-3 py-2 text-xs text-amber-800">
            ⚠️ <strong>Конфиденциальность:</strong> не указывайте в чате личные данные — ФИО, email, телефон, паспорт, ИНН и т.п. Система автоматически блокирует сообщения с персональными данными (152-ФЗ).
          </div>

          <div className="flex gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 px-4 py-2 rounded-lg border border-zinc-300 text-zinc-700 hover:bg-zinc-50"
            >
              Отмена
            </button>
            <button
              type="submit"
              disabled={!isValid}
              className={`flex-1 px-4 py-2 rounded-lg text-white ${
                isValid ? 'bg-orange-600 hover:bg-orange-700' : 'bg-zinc-300 cursor-not-allowed'
              }`}
            >
              Начать
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
