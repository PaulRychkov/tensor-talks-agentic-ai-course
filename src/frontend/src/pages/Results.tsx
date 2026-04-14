import { Link, useParams, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import ReactMarkdown from 'react-markdown'
import {
  getInterviewResult,
  getInterviewChat,
  getResults,
  submitSessionRating,
  type ChatMessage,
  type ResultsResponse,
  type ReportJSON,
  type ErrorEntry,
  type QuestionEvaluation,
  type PresetTraining,
} from '../services/chat'

function ScoreBadge({ score }: { score: number }) {
  const color =
    score >= 80 ? 'text-green-700 bg-green-50 border-green-200'
    : score >= 50 ? 'text-yellow-700 bg-yellow-50 border-yellow-200'
    : 'text-red-700 bg-red-50 border-red-200'
  return (
    <span className={`inline-block text-3xl font-bold px-4 py-2 rounded-xl border ${color}`}>
      {score}%
    </span>
  )
}

function RichReport({ report }: { report: ReportJSON }) {
  return (
    <div className="space-y-6">
      {report.summary && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">Резюме</h3>
          <div className="markdown-content text-sm text-zinc-700">
            <ReactMarkdown>{report.summary}</ReactMarkdown>
          </div>
        </div>
      )}

      {report.study_plan && report.study_plan.length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">План изучения</h3>
          <ul className="list-disc list-inside space-y-1 text-sm text-zinc-700">
            {report.study_plan.map((line, i) => <li key={i}>{line}</li>)}
          </ul>
        </div>
      )}

      {report.theory_reviewed && report.theory_reviewed.length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">Пройденная теория</h3>
          <div className="space-y-3">
            {report.theory_reviewed.map((entry, i) => (
              <details key={i} className="bg-purple-50 border border-purple-100 rounded-lg p-3">
                <summary className="cursor-pointer text-sm font-medium text-purple-800">
                  {entry.topic ? `${entry.topic} · ` : ''}{entry.question}
                </summary>
                <div className="markdown-content mt-2 text-sm text-zinc-700">
                  <ReactMarkdown>{entry.theory}</ReactMarkdown>
                </div>
              </details>
            ))}
          </div>
        </div>
      )}

      {report.strengths?.length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">Сильные стороны</h3>
          <ul className="list-disc list-inside space-y-1 text-sm text-zinc-700">
            {report.strengths.map((s, i) => <li key={i}>{s}</li>)}
          </ul>
        </div>
      )}

      {report.errors_by_topic && Object.keys(report.errors_by_topic).length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">Ошибки по темам</h3>
          <div className="space-y-3">
            {Object.entries(report.errors_by_topic).map(([topic, errors]) => (
              <div key={topic} className="bg-red-50 border border-red-100 rounded-lg p-3">
                <div className="text-sm font-medium text-red-800 mb-2">{topic}</div>
                <div className="space-y-2">
                  {errors.map((entry: ErrorEntry | string, i: number) => {
                    if (typeof entry === 'string') {
                      return <div key={i} className="text-sm text-red-700">• {entry}</div>
                    }
                    return (
                      <div key={i} className="text-sm space-y-0.5 border-l-2 border-red-300 pl-2">
                        <div className="text-red-700"><span className="font-medium">Вопрос:</span> {entry.question}</div>
                        <div className="text-red-600"><span className="font-medium">Ошибка:</span> {entry.error}</div>
                        <div className="text-green-700"><span className="font-medium">Верно:</span> {entry.correction}</div>
                      </div>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {report.preparation_plan?.length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">План подготовки</h3>
          <ol className="list-decimal list-inside space-y-1 text-sm text-zinc-700">
            {report.preparation_plan.map((item, i) => (
              <li key={i}>{typeof item === 'string' ? item : (item as any).action ?? JSON.stringify(item)}</li>
            ))}
          </ol>
        </div>
      )}

      {report.materials?.length > 0 && (
        <div>
          <h3 className="font-semibold text-zinc-800 mb-2">Рекомендуемые материалы</h3>
          <ul className="list-disc list-inside space-y-1 text-sm text-zinc-700">
            {report.materials.map((m, i) => (
              <li key={i}>{typeof m === 'string' ? m : (m as any).title ?? JSON.stringify(m)}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function EvaluationsTable({ evaluations }: { evaluations: QuestionEvaluation[] }) {
  if (!evaluations || !evaluations.length) return null
  return (
    <div>
      <h3 className="font-semibold text-zinc-800 mb-2">Оценки по вопросам</h3>
      <div className="overflow-hidden rounded-lg border border-orange-100">
        <table className="w-full text-sm">
          <thead className="bg-orange-50">
            <tr>
              <th className="text-left p-2">#</th>
              <th className="text-left p-2">Тема</th>
              <th className="text-left p-2">Решение</th>
              <th className="text-right p-2">Балл</th>
            </tr>
          </thead>
          <tbody>
            {evaluations.map((ev, i) => {
              const scoreColor = ev.score >= 7 ? 'text-green-700' : ev.score >= 4 ? 'text-yellow-700' : 'text-red-700'
              return (
                <tr key={ev.question_id || i} className="border-t border-orange-50">
                  <td className="p-2 text-zinc-500">{i + 1}</td>
                  <td className="p-2">{ev.topic || '—'}</td>
                  <td className="p-2 text-zinc-600">{ev.decision}</td>
                  <td className={`p-2 text-right font-semibold ${scoreColor}`}>{ev.score}/10</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function SessionRating({ sessionId }: { sessionId: string }) {
  const [rating, setRating] = useState(0)
  const [hovered, setHovered] = useState(0)
  const [comment, setComment] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    if (!rating) return
    setLoading(true)
    try {
      await submitSessionRating(sessionId, rating, comment)
      setSubmitted(true)
    } catch {
      // silent — non-critical
    } finally {
      setLoading(false)
    }
  }

  if (submitted) {
    return (
      <div className="text-sm text-zinc-500 text-center py-2">Спасибо за оценку!</div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="text-sm font-medium text-zinc-700">Оцените качество сессии</div>
      <div className="flex gap-1">
        {[1, 2, 3, 4, 5].map(star => (
          <button
            key={star}
            type="button"
            onClick={() => setRating(star)}
            onMouseEnter={() => setHovered(star)}
            onMouseLeave={() => setHovered(0)}
            className="text-2xl leading-none transition-colors"
          >
            <span className={(hovered || rating) >= star ? 'text-orange-400' : 'text-zinc-300'}>★</span>
          </button>
        ))}
        {rating > 0 && <span className="ml-2 text-sm text-zinc-500 self-center">{rating}/5</span>}
      </div>
      {rating > 0 && (
        <div className="space-y-2">
          <textarea
            value={comment}
            onChange={e => setComment(e.target.value)}
            placeholder="Комментарий (необязательно)"
            rows={2}
            className="w-full text-sm px-3 py-2 border border-zinc-200 rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-orange-300"
          />
          <button
            onClick={handleSubmit}
            disabled={loading}
            className="px-4 py-1.5 rounded-lg bg-orange-600 text-white text-sm hover:bg-orange-700 disabled:opacity-50"
          >
            {loading ? 'Отправка…' : 'Отправить'}
          </button>
        </div>
      )}
    </div>
  )
}

function TrainingPreset({ preset }: { preset: PresetTraining }) {
  const isStudyFollowup = preset.follow_up_kind === 'study'

  const trainingParams = new URLSearchParams({
    source: 'preset',
    preset_id: preset.preset_id || '',
    mode: 'training',
    weak_topics: preset.weak_topics.join(','),
  }).toString()

  const studyParams = new URLSearchParams({
    source: isStudyFollowup ? 'study_followup' : 'preset',
    preset_id: preset.preset_id || '',
    mode: 'study',
    weak_topics: preset.weak_topics.join(','),
  }).toString()

  return (
    <div className="bg-orange-50 border border-orange-200 rounded-lg p-4">
      <h3 className="font-semibold text-orange-800 mb-2">
        {isStudyFollowup ? 'Рекомендация: доизучить слабые темы' : 'Рекомендация: следующие шаги'}
      </h3>
      <p className="text-sm text-orange-700 mb-2">
        {isStudyFollowup
          ? 'В этих подтемах остались ошибки — начните новую сессию изучения, чтобы их закрыть:'
          : 'По результатам сессии рекомендуется работа по темам:'}
      </p>
      <div className="flex flex-wrap gap-2 mb-3">
        {preset.weak_topics.map((t) => (
          <span key={t} className="px-2 py-1 bg-orange-100 rounded text-xs font-medium text-orange-800">{t}</span>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        <Link
          to={`/dashboard?${studyParams}`}
          className="inline-block px-3 py-1.5 rounded-lg bg-orange-600 text-white text-sm hover:bg-orange-700"
        >
          {isStudyFollowup ? 'Начать доизучение' : 'Изучить тему'}
        </Link>
        {!isStudyFollowup && (
          <Link
            to={`/dashboard?${trainingParams}`}
            className="inline-block px-3 py-1.5 rounded-lg border border-orange-300 text-orange-700 text-sm hover:bg-orange-100"
          >
            Тренировка
          </Link>
        )}
      </div>
    </div>
  )
}

export default function Results() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [result, setResult] = useState<(ResultsResponse & { terminated_early?: boolean; report_json?: ReportJSON; evaluations?: QuestionEvaluation[]; preset_training?: PresetTraining; session_kind?: string }) | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const loadData = async () => {
      if (!id) return

      try {
        setIsLoading(true)
        
        const [liveResults, resultData, chatHistory] = await Promise.all([
          getResults(id).catch(() => null),
          getInterviewResult(id).catch(() => null),
          getInterviewChat(id).catch(() => [])
        ])
        
        if (liveResults || resultData) {
          const baseResult = liveResults || {
            score: resultData?.score ?? 0,
            feedback: resultData?.feedback ?? '',
            recommendations: [],
            completed_at: resultData?.updated_at ?? resultData?.created_at ?? '',
          }
          setResult({
            ...baseResult,
            terminated_early: resultData?.terminated_early,
            report_json: resultData?.report_json,
            evaluations: resultData?.evaluations,
            preset_training: resultData?.preset_training,
            session_kind: resultData?.session_kind,
          })
        }
        setMessages(chatHistory ?? [])
      } catch (error) {
        console.error('Failed to load results:', error)
        alert('Не удалось загрузить результаты. Попробуйте еще раз.')
        navigate('/dashboard')
      } finally {
        setIsLoading(false)
      }
    }

    loadData()
  }, [id, navigate])

  const sessionLabel = result?.session_kind === 'training' ? 'тренировки' : result?.session_kind === 'study' ? 'изучения' : 'интервью'

  return (
    <div className="min-h-screen bg-gradient-to-b from-orange-50 to-white">
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <Link to="/dashboard" className="text-sm text-orange-600 hover:underline">&larr; К дашборду</Link>
          <div className="font-semibold">Результаты {sessionLabel}: <span className="text-zinc-500 font-normal text-sm">{id}</span></div>
        </div>
      </header>
      <main className="max-w-5xl mx-auto px-4 py-8 grid gap-6">
        {isLoading ? (
          <section className="bg-white rounded-xl border border-orange-100 p-6">
            <div className="text-center text-zinc-500">Загрузка результатов...</div>
          </section>
        ) : result ? (
          <>
            <section className="bg-white rounded-xl border border-orange-100 p-6">
              <h2 className="text-xl font-semibold mb-4">Итоговая оценка</h2>
              {result.terminated_early && (
                <div className="mb-4 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
                  <div className="text-sm font-medium text-yellow-800">Сессия была досрочно завершена пользователем</div>
                </div>
              )}
              <div className="mb-4">
                <ScoreBadge score={result.score} />
              </div>

              {result.report_json ? (
                <RichReport report={result.report_json} />
              ) : (
                <div>
                  <div className="markdown-content text-sm text-zinc-600 mt-2">
                    <ReactMarkdown>{result.feedback}</ReactMarkdown>
                  </div>
                  {result.recommendations?.length > 0 && (
                    <div className="mt-4">
                      <div className="text-sm font-semibold text-zinc-700 mb-2">Рекомендации</div>
                      <ul className="list-disc list-inside space-y-1 text-sm text-zinc-700">
                        {result.recommendations.map((rec, idx) => (
                          <li key={idx}>
                            <div className="markdown-content inline">
                              <ReactMarkdown>{rec}</ReactMarkdown>
                            </div>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}
            </section>

            {result.evaluations && result.evaluations.length > 0 && (
              <section className="bg-white rounded-xl border border-orange-100 p-6">
                <EvaluationsTable evaluations={result.evaluations} />
              </section>
            )}

            {result.preset_training && (
              <section className="bg-white rounded-xl border border-orange-100 p-6">
                <TrainingPreset preset={result.preset_training} />
              </section>
            )}

            <section className="bg-white rounded-xl border border-orange-100 p-6">
              <SessionRating sessionId={id!} />
            </section>

            <section className="bg-white rounded-xl border border-orange-100 p-6">
              <h3 className="font-semibold mb-4">История чата</h3>
              <div className="space-y-4 max-h-96 overflow-y-auto">
                {messages.length === 0 ? (
                  <div className="text-sm text-zinc-500 text-center py-4">История чата недоступна</div>
                ) : (
                  messages.map((msg, idx) => (
                    <div 
                      key={idx} 
                      className={`p-3 rounded-lg ${
                        msg.type === 'system' 
                          ? 'bg-orange-50 border border-orange-100' 
                          : 'bg-zinc-50 border border-zinc-100'
                      }`}
                    >
                      <div className="text-xs text-zinc-500 mb-1">
                        {msg.type === 'system' ? 'Интервьюер' : 'Вы'} &middot; {new Date(msg.created_at).toLocaleString('ru-RU')}
                      </div>
                      <div className="markdown-content text-sm text-zinc-900">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </section>
          </>
        ) : (
          <section className="bg-white rounded-xl border border-orange-100 p-6">
            <div className="text-center text-zinc-500">Результаты не найдены</div>
          </section>
        )}
      </main>
    </div>
  )
}
