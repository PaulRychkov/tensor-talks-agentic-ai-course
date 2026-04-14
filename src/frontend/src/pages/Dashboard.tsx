import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useState, useEffect, useRef } from 'react'
import type { MouseEvent } from 'react'
import MVPNotification from '../components/MVPNotification'
import InterviewParamsModal, { type InterviewParams } from '../components/InterviewParamsModal'
import {
  startChat,
  getInterviews,
  getInterviewResult,
  type InterviewInfo,
  type SessionMode,
  type PresetTraining,
} from '../services/chat'
import {
  getDashboardSummary,
  getDashboardTopicProgress,
  type DashboardSummary,
  type TopicProgress,
} from '../services/dashboard'

function Card({ children }: { children: React.ReactNode }) {
  return <div className="bg-white rounded-xl border border-orange-100 shadow-soft p-5">{children}</div>
}

const SUBTOPIC_LABELS: Record<string, string> = {
  theory_tokenization: 'Токенизация',
  theory_word_embeddings: 'Word Embeddings',
  theory_attention: 'Механизм внимания',
  theory_transformer: 'Transformer',
  theory_bert: 'BERT',
  theory_gpt: 'GPT и генерация',
  theory_rnn: 'RNN',
  theory_lstm: 'LSTM',
  theory_gru: 'GRU',
  theory_fine_tuning: 'Fine-tuning',
  theory_rag: 'RAG',
  theory_rlhf: 'RLHF',
  theory_prompt_engineering: 'Prompt Engineering',
  theory_chain_of_thought: 'Chain-of-Thought',
  theory_beam_search: 'Beam Search',
  theory_positional_encoding: 'Positional Encoding',
  theory_elmo: 'ELMo',
  theory_vector_databases: 'Векторные БД',
  theory_llama: 'LLaMA',
  theory_roberta: 'RoBERTa',
  theory_t5: 'T5',
  theory_linear_regression: 'Линейная регрессия',
  theory_logistic_regression: 'Логистическая регрессия',
  theory_gradient_descent: 'Градиентный спуск',
  theory_kmeans: 'K-means',
  theory_overfitting: 'Переобучение',
  theory_cross_validation: 'Кросс-валидация',
  theory_naive_bayes: 'Наивный Байес',
}

function subtopicLabel(id: string): string {
  return SUBTOPIC_LABELS[id] || id
}

// Determine general topic from subtopics list
function guessTopicFromSubtopics(subtopics: string[]): 'classic_ml' | 'nlp' | 'llm' {
  const mlIds = ['theory_linear_regression', 'theory_logistic_regression', 'theory_gradient_descent',
    'theory_kmeans', 'theory_overfitting', 'theory_cross_validation', 'theory_naive_bayes']
  const llmIds = ['theory_gpt', 'theory_llama', 'theory_rlhf', 'theory_rag', 'theory_fine_tuning',
    'theory_prompt_engineering', 'theory_chain_of_thought', 'theory_vector_databases']

  const mlCount = subtopics.filter(s => mlIds.includes(s)).length
  const llmCount = subtopics.filter(s => llmIds.includes(s)).length

  if (mlCount > llmCount) return 'classic_ml'
  if (llmCount > mlCount) return 'llm'
  return 'nlp'
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<'learner' | 'hr'>('learner')
  const [showMVPPopup, setShowMVPPopup] = useState(false)
  const [interviews, setInterviews] = useState<InterviewInfo[]>([])
  const [isLoadingInterviews, setIsLoadingInterviews] = useState(false)
  const [userLogin, setUserLogin] = useState<string | null>(null)
  const [showParamsModal, setShowParamsModal] = useState(false)
  const [modalDefaultMode, setModalDefaultMode] = useState<SessionMode>('interview')
  const [modalDefaultTopic, setModalDefaultTopic] = useState<'classic_ml' | 'nlp' | 'llm'>('classic_ml')
  const [modalDefaultSubtopics, setModalDefaultSubtopics] = useState<string[] | undefined>(undefined)
  const [modalTitle, setModalTitle] = useState('Параметры интервью')
  const [latestPreset, setLatestPreset] = useState<PresetTraining | null | undefined>(undefined) // undefined = not loaded yet
  const [isLoadingPreset, setIsLoadingPreset] = useState(false)
  const [isStartingChat, setIsStartingChat] = useState(false)
  const [startingMode, setStartingMode] = useState<SessionMode>('interview')
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [topicProgress, setTopicProgress] = useState<TopicProgress[]>([])
  const [searchParams, setSearchParams] = useSearchParams()
  const autoStarted = useRef(false)

  // Auto-start session from URL params (e.g. from Results preset link)
  useEffect(() => {
    if (autoStarted.current) return
    const source = searchParams.get('source')
    const mode = searchParams.get('mode') as SessionMode | null
    const weakTopics = searchParams.get('weak_topics')
    if (!source || !mode || !weakTopics) return
    autoStarted.current = true
    // Clear URL params so reload doesn't re-trigger
    setSearchParams({}, { replace: true })

    const subtopics = weakTopics.split(',').filter(Boolean)
    // Determine topic from subtopic prefix (theory_rag → llm, etc.)
    const topic: 'classic_ml' | 'nlp' | 'llm' = 'llm'
    handleDirectStart(mode, topic, subtopics)
  }, [searchParams])

  const handleFeatureClick = (e?: MouseEvent) => {
    e?.preventDefault()
    e?.stopPropagation()
    setShowMVPPopup(true)
  }

  const handleLogout = async () => {
    try {
      const tokens = JSON.parse(localStorage.getItem('tt_tokens') ?? '{}')
      if (tokens?.access_token) {
        await fetch(`${import.meta.env.VITE_API_BASE_URL ?? '/api'}/auth/logout`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${tokens.access_token}` },
        })
      }
    } catch {}
    localStorage.removeItem('tt_tokens')
    localStorage.removeItem('tt_user')
    navigate('/auth', { replace: true })
  }

  const openSessionModal = (
    mode: SessionMode,
    title: string,
    topic?: 'classic_ml' | 'nlp' | 'llm',
    subtopics?: string[]
  ) => {
    setModalDefaultMode(mode)
    setModalTitle(title)
    setModalDefaultTopic(topic || 'classic_ml')
    setModalDefaultSubtopics(subtopics)
    setShowParamsModal(true)
  }

  const handleStartInterview = async (e?: MouseEvent) => {
    e?.preventDefault()
    e?.stopPropagation()
    setShowMVPPopup(false)
    openSessionModal('interview', 'Новое интервью')
  }

  const handleStartTraining = (e?: MouseEvent) => {
    e?.preventDefault()
    e?.stopPropagation()
    openSessionModal('training', 'Новая тренировка')
  }

  const handleStartStudy = (e?: MouseEvent) => {
    e?.preventDefault()
    e?.stopPropagation()
    openSessionModal('study', 'Новая тема для изучения')
  }

  const handleDirectStart = async (
    mode: SessionMode,
    topic: 'classic_ml' | 'nlp' | 'llm',
    subtopics: string[],
    level: string = 'middle',
  ) => {
    const userStr = localStorage.getItem('tt_user')
    if (!userStr) { navigate('/auth'); return }
    const user = JSON.parse(userStr)
    if (!user?.id) { navigate('/auth'); return }

    setStartingMode(mode)
    setIsStartingChat(true)
    try {
      const response = await startChat(user.id, {
        topics: [topic],
        level,
        mode,
        subtopics: subtopics.length ? subtopics : undefined,
        use_previous_results: true,
      })
      if (response?.session_id) {
        navigate(`/chat/${response.session_id}?mode=${mode}`)
      } else {
        alert('Ошибка: неверный ответ от сервера')
      }
    } catch (error: any) {
      let msg = 'Не удалось начать сессию. Попробуйте еще раз.'
      if (error?.message?.includes('already has an active session')) {
        msg = 'У вас уже есть активная сессия. Завершите текущую перед началом новой.'
      } else if (error?.message) {
        msg = error.message
      }
      alert(msg)
    } finally {
      setIsStartingChat(false)
    }
  }

  const handleConfirmParams = async (params: InterviewParams) => {
    setShowParamsModal(false)

    const userStr = localStorage.getItem('tt_user')
    if (!userStr) {
      navigate('/auth')
      return
    }

    setStartingMode(params.mode)
    setIsStartingChat(true)
    try {
      const user = JSON.parse(userStr)
      if (!user || !user.id) {
        alert('Ошибка: пользователь не найден. Пожалуйста, войдите снова.')
        navigate('/auth')
        return
      }

      const topicMap: Record<string, string[]> = {
        classic_ml: ['classic_ml'],
        nlp: ['nlp'],
        llm: ['llm'],
      }

      const response = await startChat(user.id, {
        topics: topicMap[params.topic] || ['classic_ml'],
        level: params.level,
        mode: params.mode,
        subtopics: params.subtopics?.length ? params.subtopics : undefined,
        use_previous_results: true,
        num_questions: params.num_questions,
      })

      if (response && response.session_id) {
        navigate(`/chat/${response.session_id}`)
      } else {
        alert('Ошибка: неверный ответ от сервера')
      }
    } catch (error: any) {
      console.error('Failed to start session:', error)
      let errorMessage = 'Не удалось начать сессию. Попробуйте еще раз.'
      if (error?.message?.includes('already has an active session')) {
        errorMessage = 'У вас уже есть активная сессия. Завершите текущую перед началом новой.'
      } else if (error?.message) {
        errorMessage = error.message
      }
      alert(errorMessage)
    } finally {
      setIsStartingChat(false)
    }
  }

  // Load user login
  useEffect(() => {
    try {
      const userStr = localStorage.getItem('tt_user')
      if (userStr) {
        const user = JSON.parse(userStr)
        if (user?.login) setUserLogin(user.login)
      }
    } catch (e) {}
  }, [])

  // Load dashboard summary + topic progress
  useEffect(() => {
    getDashboardSummary().then(s => setSummary(s ?? null))
    getDashboardTopicProgress().then(p => setTopicProgress(p ?? []))
  }, [])

  // Load interviews
  useEffect(() => {
    const loadInterviews = async () => {
      const userStr = localStorage.getItem('tt_user')
      if (!userStr) return
      try {
        setIsLoadingInterviews(true)
        const user = JSON.parse(userStr)
        if (user && user.id) {
          const list = await getInterviews(user.id)
          const sorted = list.sort(
            (a, b) => new Date(b.start_time).getTime() - new Date(a.start_time).getTime()
          )
          setInterviews(sorted)
        }
      } catch (error) {
        console.error('Failed to load interviews:', error)
      } finally {
        setIsLoadingInterviews(false)
      }
    }
    loadInterviews()
  }, [])

  // Load latest preset from most recent completed interview
  useEffect(() => {
    const loadPreset = async () => {
      const completed = interviews.filter(
        i => i.params.mode === 'interview' && i.end_time && i.has_results
      )
      if (completed.length === 0) {
        setLatestPreset(null)
        return
      }
      setIsLoadingPreset(true)
      try {
        const result = await getInterviewResult(completed[0].session_id)
        setLatestPreset(result?.preset_training ?? null)
      } catch {
        setLatestPreset(null)
      } finally {
        setIsLoadingPreset(false)
      }
    }
    if (interviews.length > 0) {
      loadPreset()
    } else if (!isLoadingInterviews) {
      setLatestPreset(null)
    }
  }, [interviews, isLoadingInterviews])

  const candidates = [
    { id: 'cand-1', name: 'Алексей Петров', role: 'ML Engineer', score: 72 },
    { id: 'cand-2', name: 'Мария Иванова', role: 'DS/ML', score: 81 },
  ]

  const hasPreset = latestPreset && latestPreset.weak_topics?.length > 0
  const presetLoaded = latestPreset !== undefined

  return (
    <div className="min-h-screen bg-gradient-to-b from-orange-50 to-white">
      {isStartingChat && (
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-white/80 backdrop-blur-sm">
          <div className="flex gap-1.5 mb-4">
            <span className="w-2.5 h-2.5 bg-orange-500 rounded-full animate-bounce" style={{animationDelay: '0ms'}} />
            <span className="w-2.5 h-2.5 bg-orange-500 rounded-full animate-bounce" style={{animationDelay: '150ms'}} />
            <span className="w-2.5 h-2.5 bg-orange-500 rounded-full animate-bounce" style={{animationDelay: '300ms'}} />
          </div>
          <div className="text-zinc-700 font-medium">
            Подготовка {startingMode === 'study' ? 'изучения' : startingMode === 'training' ? 'тренировки' : 'интервью'}...
          </div>
          <div className="text-sm text-zinc-500 mt-1">Это может занять несколько секунд</div>
        </div>
      )}
      <MVPNotification isOpen={showMVPPopup} onClose={() => setShowMVPPopup(false)} />
      <InterviewParamsModal
        isOpen={showParamsModal}
        onClose={() => setShowParamsModal(false)}
        onConfirm={handleConfirmParams}
        defaultMode={modalDefaultMode}
        defaultTopic={modalDefaultTopic}
        defaultSubtopics={modalDefaultSubtopics}
        title={modalTitle}
      />
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center">
          <Link to="/" className="flex items-center gap-3">
            <div className="size-8 rounded-lg bg-gradient-to-br from-orange-500 to-rose-500" />
            <div className="font-bold">TensorTalks</div>
          </Link>
          <div className="flex-1 flex justify-center">
            <div className="rounded-xl border border-orange-100 bg-white p-1 inline-flex gap-1 text-sm">
              <button
                onClick={() => setActiveTab('learner')}
                className={`px-3 py-1.5 rounded-lg ${activeTab === 'learner' ? 'bg-orange-600 text-white' : 'hover:bg-orange-50'}`}
              >
                Для обучаемого
              </button>
              <button
                onClick={() => setActiveTab('hr')}
                className={`px-3 py-1.5 rounded-lg ${activeTab === 'hr' ? 'bg-orange-600 text-white' : 'hover:bg-orange-50'}`}
              >
                Для HR/TechLead
              </button>
            </div>
          </div>
          <nav className="flex items-center gap-4 text-sm">
            {userLogin && <span className="text-zinc-600 font-medium">{userLogin}</span>}
            <button onClick={() => navigate('/account')} className="text-orange-700 hover:underline cursor-pointer">Профиль</button>
            <button onClick={() => navigate('/billing')} className="text-zinc-600 hover:underline cursor-pointer">Подписка</button>
            <button onClick={handleLogout} className="px-3 py-1.5 rounded-lg border border-orange-200 hover:bg-orange-50">Выйти</button>
          </nav>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 py-8">
        <div className="mb-6">
          <h1 className="text-2xl md:text-3xl font-bold tracking-tight">Дашборд</h1>
          <p className="text-zinc-600 mt-1">
            {activeTab === 'learner'
              ? 'Ваш личный центр подготовки к ML‑интервью: прогресс, рекомендации и быстрый старт.'
              : 'Обзор воронки кандидатов, активных интервью и аналитики компетенций команды.'}
          </p>
        </div>

        {activeTab === 'learner' && (
          <div className="grid md:grid-cols-3 gap-5 mb-6">
            <Card>
              <div className="text-sm text-zinc-500">Уровень</div>
              {summary ? (
                <>
                  <div className="text-lg font-semibold capitalize">
                    {summary.current_level || '—'}
                  </div>
                  <div className="text-sm text-zinc-500">
                    Сессий всего: {summary.total_sessions} · завершено: {summary.completed_sessions}
                  </div>
                </>
              ) : (
                <div className="text-lg font-semibold text-zinc-300 animate-pulse">—</div>
              )}
            </Card>
            <Card>
              <div className="text-sm text-zinc-500">Средний балл</div>
              {summary ? (
                <>
                  <div className="text-lg font-semibold">
                    {summary.avg_score > 0 ? `${Math.round(summary.avg_score * 100)}%` : '—'}
                  </div>
                  <div className="mt-2 h-2 bg-orange-100 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-gradient-to-r from-orange-400 to-rose-400 transition-all duration-500"
                      style={{ width: `${Math.round(summary.avg_score * 100)}%` }}
                    />
                  </div>
                  <div className="text-sm mt-1 text-zinc-500">
                    🔥 {summary.streak_days} {summary.streak_days === 1 ? 'день' : summary.streak_days < 5 ? 'дня' : 'дней'} подряд
                  </div>
                </>
              ) : (
                <div className="text-lg font-semibold text-zinc-300 animate-pulse">—</div>
              )}
            </Card>
            <Card>
              <div className="text-sm text-zinc-500">Подписка</div>
              <div className="font-semibold">Бесплатный доступ (MVP)</div>
              <button onClick={() => navigate('/billing')} className="mt-2 px-3 py-1.5 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-sm">Подробнее</button>
            </Card>
          </div>
        )}

        {activeTab === 'hr' && (
          <div className="grid md:grid-cols-3 gap-5 mb-6">
            <Card>
              <div className="text-sm text-zinc-500">Роль</div>
              <div className="text-lg font-semibold">HR · TechLead</div>
              <div className="text-sm text-zinc-500">Управление кандидатами и интервью</div>
            </Card>
            <Card>
              <div className="text-sm text-zinc-500">Сессий всего</div>
              <div className="text-lg font-semibold">{summary ? summary.total_sessions : '—'}</div>
              <div className="text-sm text-zinc-500">завершено: {summary ? summary.completed_sessions : '—'}</div>
            </Card>
            <Card>
              <div className="text-sm text-zinc-500">Подписка</div>
              <div className="font-semibold">Бесплатный доступ (MVP)</div>
              <button onClick={() => navigate('/billing')} className="mt-2 px-3 py-1.5 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-sm">Подробнее</button>
            </Card>
          </div>
        )}

        {/* Learner content */}
        {activeTab === 'learner' && (
          <section className="grid gap-6">
            {/* Quick actions */}
            <div className="flex flex-wrap gap-3">
              <button
                type="button"
                onClick={handleStartInterview}
                className="px-4 py-2 rounded-lg bg-gradient-to-r from-orange-600 to-rose-600 text-white hover:from-orange-700 hover:to-rose-700 animate-gradient-shift animate-pulse-glow"
              >
                Начать интервью
              </button>
              <button
                type="button"
                onClick={handleStartTraining}
                className="px-4 py-2 rounded-lg bg-orange-100 text-orange-800 hover:bg-orange-200 border border-orange-200"
              >
                Тренировка
              </button>
              <button
                type="button"
                onClick={handleStartStudy}
                className="px-4 py-2 rounded-lg bg-orange-100 text-orange-800 hover:bg-orange-200 border border-orange-200"
              >
                Изучение темы
              </button>
            </div>

            {/* Рекомендовано к изучению */}
            <div>
              <h2 className="text-xl font-semibold mb-3">Рекомендовано к изучению</h2>
              {isLoadingPreset || (isLoadingInterviews && presetLoaded === undefined) ? (
                <div className="text-sm text-zinc-400 py-4">Загрузка...</div>
              ) : hasPreset ? (
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                  {latestPreset!.weak_topics.map((topicId) => {
                    const guessedTopic = guessTopicFromSubtopics([topicId])
                    const label = subtopicLabel(topicId)
                    const categoryLabel = guessedTopic === 'classic_ml' ? 'Classic ML' : guessedTopic === 'llm' ? 'LLM' : 'NLP'
                    const trainingUnlocked = summary?.training_unlocked_topics?.includes(topicId)
                    return (
                      <Card key={topicId}>
                        <div className="flex flex-col gap-3">
                          <div className="flex items-center gap-2">
                            <span className="text-xl">📚</span>
                            <div className="min-w-0 flex-1">
                              <div className="text-zinc-700 font-medium text-sm leading-tight truncate">{label}</div>
                              <p className="text-xs text-zinc-400">{categoryLabel}</p>
                            </div>
                          </div>
                          <div className="flex gap-2">
                            <button
                              type="button"
                              onClick={() => handleDirectStart('study', guessedTopic, [topicId])}
                              disabled={isStartingChat}
                              className="flex-1 px-2 py-1.5 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-xs disabled:opacity-50"
                            >
                              📚 Изучить
                            </button>
                            <button
                              type="button"
                              onClick={() => trainingUnlocked && handleDirectStart('training', guessedTopic, [topicId])}
                              disabled={!trainingUnlocked || isStartingChat}
                              className={`flex-1 px-2 py-1.5 rounded-lg text-xs ${trainingUnlocked ? 'border border-orange-300 text-orange-700 hover:bg-orange-100' : 'bg-zinc-100 text-zinc-400 cursor-not-allowed'}`}
                            >
                              🏋️
                            </button>
                          </div>
                        </div>
                      </Card>
                    )
                  })}
                </div>
              ) : (
                <div className="grid md:grid-cols-2 gap-4">
                  <Card>
                    <div className="flex items-center gap-3 py-1">
                      <span className="text-2xl">🏋️</span>
                      <div className="flex-1 min-w-0">
                        <div className="text-zinc-600 font-medium text-sm">Рекомендаций пока нет</div>
                        <p className="text-xs text-zinc-400 mt-0.5">Пройдите интервью — аналитик определит слабые темы</p>
                      </div>
                      <button
                        type="button"
                        onClick={handleStartTraining}
                        className="shrink-0 px-3 py-1.5 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-xs"
                      >
                        Тренировка
                      </button>
                    </div>
                  </Card>
                  <Card>
                    <div className="flex items-center gap-3 py-1">
                      <span className="text-2xl">📚</span>
                      <div className="flex-1 min-w-0">
                        <div className="text-zinc-600 font-medium text-sm">Тем к изучению нет</div>
                        <p className="text-xs text-zinc-400 mt-0.5">После интервью аналитик предложит темы для изучения</p>
                      </div>
                      <button
                        type="button"
                        onClick={handleStartStudy}
                        className="shrink-0 px-3 py-1.5 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-xs"
                      >
                        Изучить
                      </button>
                    </div>
                  </Card>
                </div>
              )}
            </div>

            {/* Прогресс по темам — заполняется только из сессий режима «Изучение» */}
            {topicProgress.length > 0 && (
              <div>
                <h2 className="text-xl font-semibold mb-1">Прогресс по темам</h2>
                <p className="text-sm text-zinc-400 mb-3">Накопленный результат по темам из сессий режима «Изучение»</p>
                <div className="grid gap-2">
                  {topicProgress.map(tp => (
                    <div key={tp.topic} className="bg-white rounded-xl border border-orange-100 shadow-soft px-4 py-3 flex items-center gap-4">
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium text-zinc-800 truncate">{tp.topic}</div>
                        <div className="mt-1 h-1.5 bg-orange-100 rounded-full overflow-hidden">
                          <div
                            className="h-full bg-gradient-to-r from-orange-400 to-rose-400 transition-all duration-500"
                            style={{ width: `${Math.round(tp.score * 100)}%` }}
                          />
                        </div>
                      </div>
                      <div className="text-sm font-semibold text-orange-600 shrink-0">
                        {Math.round(tp.score * 100)}%
                      </div>
                      <div className={`text-xs px-2 py-0.5 rounded-full shrink-0 ${
                        tp.status === 'completed' ? 'bg-green-100 text-green-700' :
                        tp.status === 'in_progress' ? 'bg-orange-100 text-orange-700' :
                        'bg-zinc-100 text-zinc-500'
                      }`}>
                        {tp.status === 'completed' ? 'Завершено' : tp.status === 'in_progress' ? 'В процессе' : 'Не начато'}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Пройденные сессии */}
            <div>
              <h2 className="text-xl font-semibold mb-3">Пройденные сессии</h2>
              <div className="overflow-hidden rounded-xl border border-orange-100">
                <div className="max-h-96 overflow-y-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-orange-50 sticky top-0">
                      <tr>
                        <th className="text-left p-3">Темы</th>
                        <th className="text-left p-3">Режим</th>
                        <th className="text-left p-3">Дата</th>
                        <th className="text-left p-3">Статус</th>
                        <th className="text-left p-3">Оценка</th>
                        <th className="text-left p-3">Обратная связь</th>
                      </tr>
                    </thead>
                    <tbody>
                      {isLoadingInterviews ? (
                        <tr>
                          <td colSpan={6} className="p-3 text-center text-zinc-500">Загрузка...</td>
                        </tr>
                      ) : interviews.length === 0 ? (
                        <tr>
                          <td colSpan={6} className="p-3 text-center text-zinc-500">Нет пройденных сессий</td>
                        </tr>
                      ) : (
                        interviews.map((i) => {
                          const dateTime = new Date(i.start_time).toLocaleString('ru-RU', {
                            day: '2-digit',
                            month: '2-digit',
                            year: 'numeric',
                            hour: '2-digit',
                            minute: '2-digit',
                          })
                          const title = i.params.topics?.join(', ') || 'ML'
                          const modeLabel =
                            i.params.mode === 'training' ? 'Тренировка'
                            : i.params.mode === 'study' ? 'Изучение'
                            : 'Интервью'
                          const isActive = !i.end_time
                          let status = isActive ? 'Активная' : 'Завершена'
                          let statusColor = isActive ? 'text-blue-600' : 'text-green-600'
                          if (i.terminated_early) {
                            status = 'Досрочно завершена'
                            statusColor = 'text-yellow-600'
                          }
                          const feedbackDisplay =
                            i.feedback && i.feedback.length > 50
                              ? i.feedback.substring(0, 50) + '...'
                              : i.feedback || '—'

                          return (
                            <tr key={i.session_id} className="border-t border-orange-100 hover:bg-orange-50/50">
                              <td
                                className="p-3 text-orange-700 underline cursor-pointer"
                                onClick={() => {
                                  if (i.has_results || i.end_time) navigate(`/results/${i.session_id}`)
                                  else navigate(`/chat/${i.session_id}`)
                                }}
                              >
                                {title}
                              </td>
                              <td className="p-3 text-zinc-600">{modeLabel}</td>
                              <td className="p-3">{dateTime}</td>
                              <td className={`p-3 font-medium ${statusColor}`}>{status}</td>
                              <td
                                className="p-3 text-orange-700 underline cursor-pointer"
                                onClick={() => {
                                  if (i.has_results || i.end_time) navigate(`/results/${i.session_id}`)
                                }}
                              >
                                {i.has_results && i.score !== null && i.score !== undefined
                                  ? `${i.score}%`
                                  : isActive ? '—' : 'Нет оценки'}
                              </td>
                              <td className="p-3 text-zinc-600 text-sm">{feedbackDisplay}</td>
                            </tr>
                          )
                        })
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </section>
        )}

        {/* HR content */}
        {activeTab === 'hr' && (
          <section className="grid gap-6">
            <div className="flex flex-wrap gap-3">
              <button onClick={handleFeatureClick} className="px-4 py-2 rounded-lg bg-gradient-to-r from-orange-600 to-rose-600 text-white hover:from-orange-700 hover:to-rose-700">
                Создать интервью кандидату
              </button>
              <button onClick={handleFeatureClick} className="px-4 py-2 rounded-lg border border-orange-200 hover:bg-orange-50">
                Открыть отчёты по команде
              </button>
              <button onClick={handleFeatureClick} className="px-4 py-2 rounded-lg border border-orange-200 hover:bg-orange-50">
                Управление доступами
              </button>
            </div>

            <div>
              <h2 className="text-xl font-semibold mb-3">Кандидаты</h2>
              <div className="overflow-hidden rounded-xl border border-orange-100">
                <table className="w-full text-sm">
                  <thead className="bg-orange-50">
                    <tr>
                      <th className="text-left p-3">Кандидат</th>
                      <th className="text-left p-3">Роль</th>
                      <th className="text-left p-3">Сводка</th>
                    </tr>
                  </thead>
                  <tbody>
                    {candidates.map((c) => (
                      <tr key={c.id} className="border-t border-orange-100 hover:bg-orange-50/50">
                        <td className="p-3 text-orange-700 underline cursor-pointer" onClick={() => navigate(`/chat/${c.id}`)}>{c.name}</td>
                        <td className="p-3">{c.role}</td>
                        <td className="p-3 text-orange-700 underline cursor-pointer" onClick={() => navigate(`/results/${c.id}`)}>{c.score}% пригодности</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="grid md:grid-cols-3 gap-4">
              <Card>
                <div className="text-sm text-zinc-500">Калибровка интервьюеров</div>
                <div className="text-lg font-semibold">Отклонение оценок · 6%</div>
                <div className="text-sm text-zinc-500">Цель ≤ 5% • рекомендуем синхронизацию критериев</div>
              </Card>
              <Card>
                <div className="text-sm text-zinc-500">Среднее время интервью</div>
                <div className="text-lg font-semibold">38 мин</div>
                <div className="text-sm text-zinc-500">AI‑интервью экономит ~22 мин по сравнению с ручным</div>
              </Card>
              <Card>
                <div className="text-sm text-zinc-500">Конверсия этапа</div>
                <div className="text-lg font-semibold">Техническое → оффер: 18%</div>
                <div className="text-sm text-zinc-500">Следите за узкими местами по компетенциям</div>
              </Card>
            </div>
          </section>
        )}
      </main>
    </div>
  )
}
