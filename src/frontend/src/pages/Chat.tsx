import { Link, useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useState, useEffect, useRef, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import { sendMessage, pollQuestion, getResults, resumeChat, getInterviewChat, terminateChat, getInterviews, submitMessageFeedback, type ResultsResponse, type ChatMessage, type SessionMode } from '../services/chat'
import TerminateConfirmModal from '../components/TerminateConfirmModal'

type MessageType = 'question' | 'answer' | 'user' | 'system' | 'hint' | 'summary'

export interface PlanItem { title: string; subItems: string[] }

// Parse study mode markers from question content
function parseStudyContent(content: string): { type: 'plan' | 'theory_question' | 'question_only' | 'plain'; plan?: PlanItem[]; theory?: string; question?: string } {
  if (content.includes('[STUDY_PLAN]')) {
    const match = content.match(/\[STUDY_PLAN\]([\s\S]*?)\[\/STUDY_PLAN\]/)
    if (match) {
      const raw = match[1].replace(/^\n+|\n+$/g, '')
      const lines = raw.split('\n')
      const plan: PlanItem[] = []
      for (const rawLine of lines) {
        if (!rawLine.trim()) continue
        const leadingWhitespace = rawLine.match(/^[\t ]+/)?.[0] ?? ''
        const stripped = rawLine.trim()
        // Sub-item if indented OR starts with "- " / "* " / "• "
        const isSubItem = leadingWhitespace.length > 0 || /^[-*•]\s+/.test(stripped)
        const text = stripped.replace(/^[-*•]\s+/, '').trim()
        if (!text) continue
        if (isSubItem && plan.length > 0) {
          plan[plan.length - 1].subItems.push(text)
        } else {
          // Strip leading "1. " / "1) " numbering for top-level items
          const cleaned = text.replace(/^\d+[.)]\s+/, '').trim()
          plan.push({ title: cleaned, subItems: [] })
        }
      }
      return { type: 'plan', plan }
    }
  }
  if (content.includes('[STUDY_THEORY]')) {
    const theoryMatch = content.match(/\[STUDY_THEORY\]([\s\S]*?)\[\/STUDY_THEORY\]/)
    const questionMatch = content.match(/\[STUDY_QUESTION\]([\s\S]*?)\[\/STUDY_QUESTION\]/)
    return {
      type: 'theory_question',
      theory: theoryMatch?.[1]?.trim() ?? '',
      question: questionMatch?.[1]?.trim() ?? content,
    }
  }
  if (content.includes('[STUDY_QUESTION]')) {
    const questionMatch = content.match(/\[STUDY_QUESTION\]([\s\S]*?)\[\/STUDY_QUESTION\]/)
    return {
      type: 'question_only',
      question: questionMatch?.[1]?.trim() ?? content,
    }
  }
  return { type: 'plain' }
}

function MessageStars({ msgId, current, onRate }: { msgId: string; current: number; onRate: (id: string, rating: number) => void }) {
  const [hover, setHover] = useState(0)
  return (
    <div className="flex items-center gap-0.5 mt-1">
      {[1, 2, 3, 4, 5].map(s => (
        <button
          key={s}
          onClick={() => onRate(msgId, s)}
          onMouseEnter={() => setHover(s)}
          onMouseLeave={() => setHover(0)}
          className={`text-sm transition-colors ${s <= (hover || current) ? 'text-orange-400' : 'text-zinc-300'}`}
          title={`Оценить: ${s}`}
        >★</button>
      ))}
    </div>
  )
}

interface Message {
  id: string
  type: MessageType
  content: string
  timestamp: string
  responseTimeMs?: number
  piiMasked?: boolean
}

export default function Chat() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [messages, setMessages] = useState<Message[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [feedbackText, setFeedbackText] = useState('')
  const [feedbackSent, setFeedbackSent] = useState(false)
  const [sessionId] = useState<string | null>(id || null)
  const [userId, setUserId] = useState<string | null>(null)
  const [chatCompleted, setChatCompleted] = useState(false)
  const [results, setResults] = useState<ResultsResponse | null>(null)
  const [showTerminateModal, setShowTerminateModal] = useState(false)
  const modeFromUrl = searchParams.get('mode') as SessionMode | null
  const [sessionMode, setSessionMode] = useState<SessionMode | null>(modeFromUrl)
  const [sessionTopicLabel, setSessionTopicLabel] = useState<string>('')
  const [revealedQuestions, setRevealedQuestions] = useState<Set<string>>(new Set())
  const [totalQuestions, setTotalQuestions] = useState<number>(0)
  const [currentQuestionNumber, setCurrentQuestionNumber] = useState<number>(0)
  const [waitingSince, setWaitingSince] = useState<number | null>(null)
  const [waitingSeconds, setWaitingSeconds] = useState(0)
  const [agentStep, setAgentStep] = useState<string>('')   // current backend processing step
  const [stepLabel, setStepLabel] = useState<string>('')   // animated label shown to user
  const [messageRatings, setMessageRatings] = useState<Record<string, number>>({}) // questionId → 1-5
  const [pendingSince, setPendingSince] = useState<number | null>(null) // timestamp when pending started
  const [pendingSeconds, setPendingSeconds] = useState(0)
  const pollingIntervalRef = useRef<number | null>(null)
  const resultsPollingRef = useRef<number | null>(null)
  const waitingSinceRef = useRef<number | null>(null)
  const hasResumedRef = useRef(false)
  const chatContainerRef = useRef<HTMLDivElement | null>(null)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const resultsRef = useRef<HTMLDivElement | null>(null)
  const resultsScrollTimeoutRef = useRef<number | null>(null)
  const isSendingRef = useRef(false)

  useEffect(() => {
    if (!waitingSince) {
      setWaitingSeconds(0)
      return
    }
    const timerId = window.setInterval(() => {
      setWaitingSeconds(Math.floor((Date.now() - waitingSince) / 1000))
    }, 250)
    return () => clearInterval(timerId)
  }, [waitingSince])

  useEffect(() => {
    waitingSinceRef.current = waitingSince
  }, [waitingSince])

  useEffect(() => {
    if (chatCompleted) {
      setWaitingSince(null)
    }
  }, [chatCompleted])

  // Track how long analyst has been generating (to show fallback link after 3 min)
  useEffect(() => {
    if (!pendingSince) {
      setPendingSeconds(0)
      return
    }
    const t = window.setInterval(() => {
      setPendingSeconds(Math.floor((Date.now() - pendingSince) / 1000))
    }, 5000)
    return () => clearInterval(t)
  }, [pendingSince])

  // Show real processing step from backend (written to Redis by agent nodes)
  useEffect(() => {
    setStepLabel(agentStep || '')
  }, [agentStep])

  const startPolling = useCallback(() => {
    if (!sessionId || chatCompleted) return

    // Очищаем предыдущий интервал если есть
    if (pollingIntervalRef.current) {
      clearInterval(pollingIntervalRef.current)
    }

    // Polling каждые 1 секунду для получения вопросов
    pollingIntervalRef.current = window.setInterval(async () => {
      const currentSessionId = sessionId
      if (!currentSessionId || chatCompleted) {
        if (pollingIntervalRef.current) {
          clearInterval(pollingIntervalRef.current)
        }
        return
      }

      try {
        // Проверяем результаты чата
        const chatResults = await getResults(currentSessionId)
        if (chatResults) {
          setIsLoading(false)
          setWaitingSince(null)
          if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current)
          }

          if (chatResults.pending) {
            // 1. Load history first (includes final model message)
            try {
              const chatHistory = await getInterviewChat(currentSessionId)
              if (chatHistory && chatHistory.length > 0) {
                setMessages(mapHistoryMessages(chatHistory))
              }
            } catch { /* ignore */ }
            // 2. Defer setChatCompleted so the final message renders + scrolls to bottom first,
            //    then the chatCompleted useEffect triggers the 2s → scroll-to-top timer
            window.setTimeout(() => setChatCompleted(true), 50)
            setPendingSince(prev => prev ?? Date.now())
            // 3. Start results polling
            if (resultsPollingRef.current) clearInterval(resultsPollingRef.current)
            resultsPollingRef.current = window.setInterval(async () => {
              try {
                const finalResults = await getResults(currentSessionId)
                if (finalResults && !finalResults.pending) {
                  setResults(finalResults)
                  if (resultsPollingRef.current) clearInterval(resultsPollingRef.current)
                }
              } catch { /* ignore */ }
            }, 3000)
            return
          }

          // Full results already available — load history then show
          try {
            const chatHistory = await getInterviewChat(currentSessionId)
            if (chatHistory && chatHistory.length > 0) {
              setMessages(mapHistoryMessages(chatHistory))
            }
          } catch (error) {
            console.warn('Failed to load chat history after completion:', error)
          }
          window.setTimeout(() => {
            setChatCompleted(true)
            setResults(chatResults)
          }, 50)
          return
        }

        // Получаем следующий вопрос
        const { question, processingStep } = await pollQuestion(currentSessionId)
        setAgentStep(processingStep)
        if (question) {
          const responseTimeMs = waitingSinceRef.current ? Date.now() - waitingSinceRef.current : undefined
          const questionMessage: Message = {
            id: question.question_id,
            type: 'question',
            content: question.question,
            timestamp: question.timestamp,
            responseTimeMs,
          }
          setMessages((prev) => {
            if (prev.some(m => m.id === question.question_id)) {
              return prev
            }
            // Study-mode content-based dedup: plan/theory are also persisted to chat-crud for
            // resume. When polling delivers them right after history load, IDs differ
            // (history-question-N vs kafka UUID) but content is identical — skip to avoid
            // duplicate plan/theory blocks on resume.
            const content = question.question
            if (content.includes('[STUDY_PLAN]')) {
              if (prev.some(m => m.type === 'question' && m.content.includes('[STUDY_PLAN]'))) {
                return prev
              }
            }
            if (content.includes('[STUDY_THEORY]')) {
              const theoryMatch = content.match(/\[STUDY_THEORY\]([\s\S]*?)\[\/STUDY_THEORY\]/)
              const theoryBody = theoryMatch?.[1]?.trim() ?? ''
              if (theoryBody && prev.some(m => m.type === 'question' && m.content.includes(theoryBody))) {
                return prev
              }
            }
            // If agent detected PII, replace content of last user message with masked version
            if (question.pii_masked_content) {
              const lastUserIdx = prev.map((_, i) => i).filter(i => prev[i].type === 'user').pop()
              if (lastUserIdx !== undefined) {
                const updated = [...prev]
                updated[lastUserIdx] = { ...updated[lastUserIdx], content: question.pii_masked_content, piiMasked: true }
                return [...updated, questionMessage]
              }
            }
            return [...prev, questionMessage]
          })
          if (question.question_number > 0) setCurrentQuestionNumber(question.question_number)
          if (question.total_questions > 0) setTotalQuestions(question.total_questions)
          setIsLoading(false)
          setWaitingSince(null)
          setAgentStep('')
        }
        // Если вопросов/результатов нет (404) - это нормально, просто продолжаем polling
      } catch (error: any) {
        // Подавляем логирование для тихих ошибок (404 на polling endpoints)
        if (error.silent || error.status === 404) {
          // Тихая ошибка - не логируем
          return;
        }
        // Логируем только реальные ошибки (не 404)
        if (!error.message?.includes('404') && !error.message?.includes('no new questions') && !error.message?.includes('chat not completed')) {
          console.error('Polling error:', error)
        }
      }
    }, 1000)
  }, [sessionId, chatCompleted])

  const mapHistoryMessages = useCallback((chatHistory: ChatMessage[]) => {
    return chatHistory.map((msg, idx) => {
      const createdAt = msg.created_at || new Date().toISOString()
      if (msg.type === 'user') {
        return {
          id: `history-user-${idx}`,
          type: 'user' as MessageType,
          content: msg.content,
          timestamp: createdAt,
        }
      }
      if (msg.type === 'system') {
        const content = msg.content || ''
        // Study mode markers → render as question type so parseStudyContent can process them
        if (content.includes('[STUDY_PLAN]') || content.includes('[STUDY_THEORY]') || content.includes('[STUDY_QUESTION]')) {
          return {
            id: `history-question-${idx}`,
            type: 'question' as MessageType,
            content,
            timestamp: createdAt,
          }
        }
        if (content.startsWith('Вопрос: ')) {
          return {
            id: `history-question-${idx}`,
            type: 'question' as MessageType,
            content: content.substring('Вопрос: '.length),
            timestamp: createdAt,
          }
        }
        if (content.startsWith('Подсказка: ') || content.startsWith('Hint: ')) {
          return {
            id: `history-hint-${idx}`,
            type: 'hint' as MessageType,
            content: content.replace(/^(Подсказка|Hint): /, ''),
            timestamp: createdAt,
          }
        }
        if (content.startsWith('Сводка: ') || content.startsWith('Summary: ')) {
          return {
            id: `history-summary-${idx}`,
            type: 'summary' as MessageType,
            content: content.replace(/^(Сводка|Summary): /, ''),
            timestamp: createdAt,
          }
        }
        return {
          id: `history-system-${idx}`,
          type: 'system' as MessageType,
          content,
          timestamp: createdAt,
        }
      }
      return {
        id: `history-question-${idx}`,
        type: 'question' as MessageType,
        content: msg.content,
        timestamp: createdAt,
      }
    })
  }, [])

  useEffect(() => {
    // Получаем user_id из localStorage
    const userStr = localStorage.getItem('tt_user')
    if (!userStr) {
      navigate('/auth')
      return
    }

    const user = JSON.parse(userStr)
    setUserId(user.id)

    // Load session mode + topic info for header display
    if (sessionId && user.id) {
      getInterviews(user.id).then((list) => {
        const found = list.find((i) => i.session_id === sessionId)
        const mode = found?.params?.mode || found?.params?.type
        if (mode) setSessionMode(mode as SessionMode)
        // Build "LLM / RAG" label from session params
        const topicMap: Record<string, string> = { llm: 'LLM', nlp: 'NLP', classic_ml: 'Classic ML' }
        const subtopicMap: Record<string, string> = {
          theory_rag: 'RAG', theory_attention: 'Внимание', theory_transformer: 'Transformer',
          theory_bert: 'BERT', theory_gpt: 'GPT', theory_rnn: 'RNN', theory_lstm: 'LSTM',
          theory_fine_tuning: 'Fine-tuning', theory_rlhf: 'RLHF', theory_prompt_engineering: 'Prompt Engineering',
          theory_chain_of_thought: 'Chain-of-Thought', theory_vector_databases: 'Векторные БД',
          theory_tokenization: 'Токенизация', theory_word_embeddings: 'Word Embeddings',
          theory_gru: 'GRU', theory_beam_search: 'Beam Search', theory_positional_encoding: 'Positional Encoding',
          theory_elmo: 'ELMo', theory_llama: 'LLaMA', theory_roberta: 'RoBERTa', theory_t5: 'T5',
          theory_linear_regression: 'Линейная регрессия', theory_logistic_regression: 'Логистическая регрессия',
          theory_gradient_descent: 'Градиентный спуск', theory_kmeans: 'K-means',
          theory_overfitting: 'Переобучение', theory_cross_validation: 'Кросс-валидация',
          theory_naive_bayes: 'Наивный Байес',
        }
        const topics: string[] = found?.params?.topics || []
        const weakTopics: string[] = found?.params?.subtopics || []
        const parts: string[] = []
        if (topics.length) parts.push(topicMap[topics[0]] || topics[0].toUpperCase())
        if (weakTopics.length) parts.push(subtopicMap[weakTopics[0]] || weakTopics[0])
        if (parts.length) setSessionTopicLabel(parts.join(' · '))
      }).catch(() => {})
    }

    // Если сессии нет, создаём новую
    if (!sessionId) {
      startNewChat()
    } else {
      if (!hasResumedRef.current) {
        hasResumedRef.current = true
        // Восстанавливаем активную сессию - загружаем историю чата и отправляем событие восстановления
        const resumeActiveSession = async () => {
          try {
            // Загружаем историю чата (BFF сам определит активный или завершенный)
            try {
              const chatHistory = await getInterviewChat(sessionId)
              if (chatHistory && chatHistory.length > 0) {
                const mappedHistory = mapHistoryMessages(chatHistory)
                setMessages(mappedHistory)
                console.log('Chat history loaded:', mappedHistory.length, 'messages')
                setWaitingSince(null)
                // История есть — это resume, снимаем лоадинг
                setIsLoading(false)
                // Auto-reveal study plan/theory messages that already have user responses after them
                const toReveal = new Set<string>()
                for (let i = 0; i < mappedHistory.length; i++) {
                  const m = mappedHistory[i]
                  if (m.type === 'question') {
                    const parsed = parseStudyContent(m.content)
                    if (parsed.type === 'plan' || parsed.type === 'theory_question') {
                      // Reveal if any user message follows this one
                      const hasUserAfter = mappedHistory.slice(i + 1).some(n => n.type === 'user')
                      if (hasUserAfter) toReveal.add(m.id)
                    }
                  }
                }
                if (toReveal.size > 0) setRevealedQuestions(toReveal)
              }
              // Для нового чата (пустая история) — оставляем isLoading=true,
              // polling сам сбросит его когда придёт первый вопрос
            } catch (error) {
              console.warn('Failed to load chat history, continuing without it:', error)
              // Не сбрасываем isLoading — polling подхватит
              // Продолжаем даже если история не загрузилась (может быть новая сессия)
              // Но все равно отправляем событие восстановления - сессия может быть активна
            }
            
            // Отправляем событие восстановления в Kafka (даже если истории нет - сессия может быть активна)
            try {
              await resumeChat(sessionId)
              console.log('Chat session resumed')
            } catch (error) {
              console.warn('Failed to send resume event, continuing:', error)
              // Продолжаем - сессия может быть уже активна или завершена
            }
          } catch (error) {
            console.error('Failed to resume chat session:', error)
            // Продолжаем даже если не удалось отправить событие восстановления
            // (сессия может быть уже завершена)
          }
        }
        resumeActiveSession()
      }
      if (messages.length === 0) {
        setWaitingSince(Date.now())
      }
    }

    return () => {
      // Очищаем polling при размонтировании
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current)
      }
      if (resultsPollingRef.current) {
        clearInterval(resultsPollingRef.current)
      }
    }
  }, [sessionId, navigate, mapHistoryMessages])

  useEffect(() => {
    if (!sessionId) return
    startPolling()
    return () => {
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current)
      }
    }
  }, [sessionId, chatCompleted, startPolling])

  // Scroll to bottom when new messages arrive, but only during active chat
  useEffect(() => {
    if (!chatCompleted) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
    }
  }, [messages, chatCompleted])

  // On completion: show final message first (instant scroll to bottom),
  // then after 2s scroll to top so the waiting/results block is visible
  useEffect(() => {
    if (chatCompleted) {
      // Immediately scroll to bottom so final model message is visible
      bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
      // After 2s, scroll to top to reveal the results/waiting block
      if (resultsScrollTimeoutRef.current) clearTimeout(resultsScrollTimeoutRef.current)
      resultsScrollTimeoutRef.current = window.setTimeout(() => {
        chatContainerRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
      }, 2000)
    }
    return () => {
      if (resultsScrollTimeoutRef.current) clearTimeout(resultsScrollTimeoutRef.current)
    }
  }, [chatCompleted])

  const startNewChat = () => {
    // Если нет сессии, перенаправляем на дашборд для выбора параметров
    navigate('/dashboard')
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !sessionId || !userId || isLoading || chatCompleted || isSendingRef.current) return

    isSendingRef.current = true
    const userMessage: Message = {
      id: Date.now().toString(),
      type: 'user',
      content: inputValue,
      timestamp: new Date().toISOString(),
    }

    setMessages((prev) => [...prev, userMessage])
    setInputValue('')
    setIsLoading(true)
    setWaitingSince(Date.now())

    try {
      await sendMessage(sessionId, inputValue)
      // Polling автоматически получит следующий вопрос или результаты
    } catch (error: any) {
      setIsLoading(false)
      setWaitingSince(null)
      // 422 — PII обнаружен в BFF до отправки в Kafka
      if (error?.status === 422 || error?.pii_detected) {
        const piiMsg: Message = {
          id: Date.now().toString() + '-pii',
          type: 'system',
          content: error?.error || 'Пожалуйста, не указывайте персональные данные (email, телефон, паспорт и т.п.) — требование 152-ФЗ. Перефразируйте ответ.',
          timestamp: new Date().toISOString(),
        }
        setMessages(prev => [...prev, piiMsg])
      } else {
        console.error('Failed to send message:', error)
        alert('Не удалось отправить сообщение. Попробуйте еще раз.')
      }
    } finally {
      isSendingRef.current = false
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleTerminate = async () => {
    if (!sessionId || !userId || chatCompleted) return
    setShowTerminateModal(true)
  }

  const handleTerminateConfirm = async () => {
    if (!sessionId || !userId || chatCompleted) return

    try {
      await terminateChat(sessionId)

      // Закрываем модальное окно только после успешного завершения
      setShowTerminateModal(false)

      // Останавливаем polling
      if (pollingIntervalRef.current) clearInterval(pollingIntervalRef.current)
      setIsLoading(false)
      setWaitingSince(null)

      // Load history so final message is visible, then switch to completed state
      try {
        const chatHistory = await getInterviewChat(sessionId)
        if (chatHistory && chatHistory.length > 0) {
          setMessages(mapHistoryMessages(chatHistory))
        }
      } catch { /* ignore */ }
      window.setTimeout(() => setChatCompleted(true), 50)

      // Poll for results (analyst takes ~30-60s)
      if (resultsPollingRef.current) clearInterval(resultsPollingRef.current)
      resultsPollingRef.current = window.setInterval(async () => {
        try {
          const chatResults = await getResults(sessionId)
          if (chatResults && !chatResults.pending) {
            setResults(chatResults)
            if (resultsPollingRef.current) clearInterval(resultsPollingRef.current)
          }
        } catch (error) {
          console.error('Failed to get results after termination:', error)
        }
      }, 3000)
    } catch (error) {
      console.error('Failed to terminate chat:', error)
      // Не закрываем модальное окно при ошибке - пользователь может попробовать снова
      // Ошибка уже логируется в консоли, пользователь увидит проблему через UI
    }
  }

  const handleMessageRating = useCallback((questionId: string, rating: number) => {
    setMessageRatings(prev => ({ ...prev, [questionId]: rating }))
    if (sessionId) submitMessageFeedback(sessionId, questionId, rating)
  }, [sessionId])

  const formatTime = (isoTime: string) => {
    try {
      return new Date(isoTime).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
    } catch {
      return isoTime
    }
  }

  const formatResponseTime = (ms?: number) => {
    if (ms === undefined) return ''
    const seconds = ms / 1000
    return seconds < 1 ? `${Math.round(ms)}мс` : `${seconds.toFixed(1)}с`
  }

  return (
    <div className="min-h-screen bg-gradient-to-b from-orange-50 to-white">
      <TerminateConfirmModal
        isOpen={showTerminateModal}
        onClose={() => setShowTerminateModal(false)}
        onConfirm={handleTerminateConfirm}
        sessionMode={sessionMode}
      />
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <Link to="/dashboard" className="text-sm text-orange-600 hover:underline">
            ← К дашборду
          </Link>
          <div className="font-semibold flex items-center gap-2 flex-wrap">
            {sessionMode && (
              <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                sessionMode === 'training' ? 'bg-blue-100 text-blue-700' :
                sessionMode === 'study' ? 'bg-purple-100 text-purple-700' :
                'bg-orange-100 text-orange-700'
              }`}>
                {sessionMode === 'training' ? 'Тренировка' : sessionMode === 'study' ? 'Изучение' : 'Интервью'}
              </span>
            )}
            {sessionTopicLabel && (() => {
              const parts = sessionTopicLabel.split(' · ')
              const badgeClass =
                sessionMode === 'training' ? 'bg-blue-50 text-blue-700 border border-blue-200' :
                sessionMode === 'study' ? 'bg-purple-50 text-purple-700 border border-purple-200' :
                'bg-orange-50 text-orange-700 border border-orange-200'
              return (
                <>
                  {parts.map((p, i) => (
                    <span
                      key={i}
                      className={`text-xs px-2 py-0.5 rounded-full font-medium ${badgeClass}`}
                    >
                      {p}
                    </span>
                  ))}
                </>
              )
            })()}
            {currentQuestionNumber > 0 && (
              <span className="text-xs text-zinc-400 font-normal">
                · {sessionMode === 'study' ? 'Пункт' : 'Вопрос'} {currentQuestionNumber}{totalQuestions > 0 ? ` из ${totalQuestions}` : ''}
              </span>
            )}
            {sessionId ? (
              <span className="text-xs text-zinc-400 font-normal"><span className="font-sans">· сессия </span><span className="font-mono">{sessionId}</span></span>
            ) : 'Создание сессии...'}
          </div>
        </div>
      </header>
      <main className="max-w-4xl mx-auto px-4 py-8 flex flex-col gap-4 h-[90vh]">
        <div className="bg-white rounded-xl border border-orange-100 p-4 flex-1 overflow-y-auto" ref={chatContainerRef}>
          <div className="text-sm text-zinc-500 mb-2">{sessionMode === 'study' ? 'Режим изучения' : sessionMode === 'training' ? 'Режим тренировки' : 'Чат с AI‑интервьюером'}</div>
          {waitingSince && (
            <div className="text-xs text-zinc-500 mb-2">
              Ожидание ответа: {waitingSeconds}с
            </div>
          )}
          <div className="space-y-4">
            {/* --- completion block at TOP --- */}
            {chatCompleted && !results && (
              <div className="bg-yellow-50 rounded-xl p-4 border border-yellow-200 text-center">
                <div className="text-yellow-800 font-semibold mb-1">{sessionMode === 'study' ? 'Изучение завершено' : sessionMode === 'training' ? 'Тренировка завершена' : 'Интервью завершено'}</div>
                <div className="text-sm text-yellow-700">Аналитик формирует отчёт, результаты появятся здесь автоматически...</div>
                <div className="flex justify-center gap-1 mt-3">
                  <span className="w-1.5 h-1.5 bg-yellow-500 rounded-full animate-bounce" style={{animationDelay: '0ms'}} />
                  <span className="w-1.5 h-1.5 bg-yellow-500 rounded-full animate-bounce" style={{animationDelay: '150ms'}} />
                  <span className="w-1.5 h-1.5 bg-yellow-500 rounded-full animate-bounce" style={{animationDelay: '300ms'}} />
                </div>
                {pendingSeconds >= 180 && sessionId && (
                  <div className="mt-3 pt-3 border-t border-yellow-200">
                    <div className="text-xs text-yellow-600 mb-2">Формирование занимает дольше обычного</div>
                    <button
                      onClick={() => navigate(`/results/${sessionId}`)}
                      className="px-3 py-1.5 rounded-lg bg-yellow-600 text-white text-xs hover:bg-yellow-700"
                    >
                      Проверить результаты
                    </button>
                  </div>
                )}
              </div>
            )}
            {chatCompleted && results && (
              <div ref={resultsRef} className="bg-gradient-to-r from-orange-50 to-rose-50 rounded-xl p-6 border border-orange-200">
                <h3 className="text-xl font-semibold mb-4 text-orange-900">{sessionMode === 'study' ? 'Итоги изучения' : sessionMode === 'training' ? 'Итоги тренировки' : 'Отчёт по интервью'}</h3>
                <div className="space-y-3">
                  <div>
                    <div className="text-sm text-zinc-600">Оценка</div>
                    <div className="text-3xl font-bold text-orange-600">{results.score}%</div>
                  </div>
                  <div>
                    <div className="text-sm text-zinc-600">Обратная связь</div>
                    <div className="markdown-content text-zinc-800">
                      <ReactMarkdown>{results.feedback}</ReactMarkdown>
                    </div>
                  </div>
                  {results.recommendations?.length > 0 && (
                    <div>
                      <div className="text-sm text-zinc-600 mb-2">Рекомендации</div>
                      <ul className="list-disc list-inside space-y-1 text-zinc-800">
                        {results.recommendations.map((rec, idx) => (
                          <li key={idx}>{rec}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                  <div className="pt-3 border-t border-orange-200">
                    <div className="text-sm text-zinc-600 mb-2">{sessionMode === 'study' ? 'Ваши впечатления от сессии (необязательно)' : 'Ваши впечатления от интервью (необязательно)'}</div>
                    {feedbackSent ? (
                      <div className="text-sm text-green-700 bg-green-50 rounded-lg px-3 py-2">Спасибо за отзыв!</div>
                    ) : (
                      <div className="flex flex-col gap-2">
                        <textarea
                          className="w-full px-3 py-2 rounded-lg border border-orange-200 focus:outline-none focus:ring-2 focus:ring-orange-400 text-sm resize-none"
                          rows={3}
                          placeholder="Что понравилось или что можно улучшить?"
                          value={feedbackText}
                          onChange={e => setFeedbackText(e.target.value)}
                        />
                        <button
                          onClick={() => { if (feedbackText.trim()) setFeedbackSent(true) }}
                          disabled={!feedbackText.trim()}
                          className="self-start px-4 py-1.5 rounded-lg bg-orange-600 text-white text-sm hover:bg-orange-700 disabled:bg-orange-200 disabled:cursor-not-allowed"
                        >
                          Отправить отзыв
                        </button>
                      </div>
                    )}
                  </div>
                </div>
                <div className="flex gap-3 mt-4">
                  <button
                    onClick={() => navigate(`/results/${sessionId}`)}
                    className="px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700 text-sm"
                  >
                    Полный отчёт
                  </button>
                  <button
                    onClick={() => navigate('/dashboard')}
                    className="px-4 py-2 rounded-lg border border-orange-300 text-orange-700 hover:bg-orange-50 text-sm"
                  >
                    К дашборду
                  </button>
                </div>
              </div>
            )}
            {/* --- separator --- */}
            {messages.length === 0 && !isLoading && !chatCompleted && (
              <div className="text-center text-zinc-400 py-8">
                {sessionMode === 'study' ? 'Сессия изучения началась. Готовим материал...' : sessionMode === 'training' ? 'Тренировка началась. Ожидайте первого вопроса...' : 'Чат начат. Ожидайте первого вопроса от AI-интервьюера...'}
              </div>
            )}
            {/* --- chat messages --- */}
            {(() => {
              // Study mode: gate messages after an unrevealed plan.
              // The plan must be confirmed via "Продолжить" before subsequent messages appear.
              if (sessionMode === 'study') {
                const planIdx = messages.findIndex(m => m.type === 'question' && parseStudyContent(m.content).type === 'plan')
                if (planIdx !== -1) {
                  const planMsg = messages[planIdx]
                  if (!revealedQuestions.has(planMsg.id)) {
                    return messages.slice(0, planIdx + 1)
                  }
                }
              }
              return messages
            })().map((msg, _visIdx, visMessages) => (
              <div key={msg.id} className="grid gap-2">
                {msg.type === 'question' && (() => {
                  const parsed = sessionMode === 'study' ? parseStudyContent(msg.content) : { type: 'plain' as const }

                  if (parsed.type === 'plan') {
                    const isRevealed = revealedQuestions.has(msg.id)
                    const planItems = parsed.plan!
                    // Count total sub-items (actual study points) for the header
                    const totalPoints = planItems.reduce((sum, item) => sum + Math.max(item.subItems.length, 1), 0)
                    const topicWord = planItems.length === 1 ? 'тема' : planItems.length < 5 ? 'темы' : 'тем'
                    return (
                      <div className="self-start max-w-[90%]">
                        <div className="rounded-2xl px-4 py-3 bg-purple-50 border border-purple-200 text-purple-900">
                          <div className="text-xs uppercase tracking-wide text-purple-600 mb-2">📋 План изучения — {planItems.length} {topicWord}, {totalPoints} {totalPoints === 1 ? 'пункт' : totalPoints < 5 ? 'пункта' : 'пунктов'}</div>
                          <ol className="space-y-2">
                            {planItems.map((item, i) => (
                              <li key={i} className="text-sm">
                                <div className="flex gap-2">
                                  <span className="text-purple-400 font-semibold shrink-0">{i + 1}.</span>
                                  <span className="font-medium">{item.title}</span>
                                </div>
                                {item.subItems.length > 0 && (
                                  <ul className="mt-1 ml-6 space-y-0.5 list-disc list-outside text-purple-800/90">
                                    {item.subItems.map((sub, j) => (
                                      <li key={j} className="text-xs leading-snug">{sub}</li>
                                    ))}
                                  </ul>
                                )}
                              </li>
                            ))}
                          </ol>
                        </div>
                        <div className="flex items-center gap-3 mt-2">
                          <span className="text-xs text-zinc-500">{formatTime(msg.timestamp)}</span>
                          {!isRevealed && (
                            <button
                              onClick={() => setRevealedQuestions(prev => { const s = new Set(prev); s.add(msg.id); return s })}
                              className="px-4 py-1.5 rounded-lg bg-purple-600 text-white text-sm hover:bg-purple-700 transition-colors"
                            >
                              Продолжить →
                            </button>
                          )}
                        </div>
                      </div>
                    )
                  }

                  if (parsed.type === 'theory_question') {
                    const isRevealed = revealedQuestions.has(msg.id)
                    // Blur only the currently-active theory block (the LAST theory_question in chat).
                    // Previously-answered blocks should stay clear.
                    let isCurrentTheory = false
                    for (let i = visMessages.length - 1; i >= 0; i--) {
                      const m = visMessages[i]
                      if (m.type === 'question' && parseStudyContent(m.content).type === 'theory_question') {
                        isCurrentTheory = m.id === msg.id
                        break
                      }
                    }
                    const shouldBlur = isRevealed && isCurrentTheory
                    return (
                      <div className="self-start max-w-[85%]">
                        <div className="flex items-start gap-2">
                          <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                            <span className="text-purple-600 text-xs font-bold">AI</span>
                          </div>
                          <div className="flex flex-col gap-2 min-w-0">
                            <div className={`rounded-2xl px-4 py-3 bg-purple-50 border border-purple-200 text-purple-900 transition-all duration-500 ${shouldBlur ? 'opacity-30 blur-sm pointer-events-none select-none' : ''}`}>
                              <div className="text-xs uppercase tracking-wide text-purple-600 mb-2">📖 Теория</div>
                              <div className="markdown-content">
                                <ReactMarkdown>{parsed.theory!}</ReactMarkdown>
                              </div>
                            </div>
                            {!isRevealed && (
                              <button
                                onClick={() => setRevealedQuestions(prev => { const s = new Set(prev); s.add(msg.id); return s })}
                                className="self-start px-4 py-2 rounded-lg bg-purple-600 text-white text-sm hover:bg-purple-700 transition-colors"
                              >
                                Продолжить →
                              </button>
                            )}
                            {isRevealed && (
                              <div className="rounded-2xl px-4 py-2 bg-orange-100 text-zinc-900">
                                <div className="text-xs uppercase tracking-wide text-orange-600 mb-1">❓ Вопрос</div>
                                <div className="markdown-content">
                                  <ReactMarkdown>{parsed.question!}</ReactMarkdown>
                                </div>
                              </div>
                            )}
                            <div className="flex items-center gap-3">
                              <span className="text-xs text-zinc-500">
                                {formatTime(msg.timestamp)}{msg.responseTimeMs !== undefined ? ` • ${formatResponseTime(msg.responseTimeMs)}` : ''}
                              </span>
                              {isRevealed && <MessageStars msgId={msg.id} current={messageRatings[msg.id] ?? 0} onRate={handleMessageRating} />}
                            </div>
                          </div>
                        </div>
                      </div>
                    )
                  }

                  if (parsed.type === 'question_only') {
                    return (
                      <div className="self-start max-w-[85%]">
                        <div className="flex items-start gap-2">
                          <div className="w-7 h-7 rounded-full bg-orange-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                            <span className="text-orange-600 text-xs font-bold">AI</span>
                          </div>
                          <div className="flex flex-col gap-2 min-w-0">
                            <div className="rounded-2xl px-4 py-2 bg-orange-100 text-zinc-900">
                              <div className="text-xs uppercase tracking-wide text-orange-600 mb-1">❓ Вопрос</div>
                              <div className="markdown-content">
                                <ReactMarkdown>{parsed.question!}</ReactMarkdown>
                              </div>
                            </div>
                            <div className="flex items-center gap-3">
                              <span className="text-xs text-zinc-500">
                                {formatTime(msg.timestamp)}{msg.responseTimeMs !== undefined ? ` • ${formatResponseTime(msg.responseTimeMs)}` : ''}
                              </span>
                              <MessageStars msgId={msg.id} current={messageRatings[msg.id] ?? 0} onRate={handleMessageRating} />
                            </div>
                          </div>
                        </div>
                      </div>
                    )
                  }

                  // plain (non-study or no markers)
                  return (
                    <div className="self-start max-w-[80%]">
                      <div className="flex items-start gap-2">
                        <div className="w-7 h-7 rounded-full bg-orange-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                          <span className="text-orange-600 text-xs font-bold">AI</span>
                        </div>
                        <div>
                          <div className="rounded-2xl px-4 py-2 bg-orange-100 text-zinc-900">
                            <div className="markdown-content">
                              <ReactMarkdown>{msg.content}</ReactMarkdown>
                            </div>
                          </div>
                          <div className="flex items-center gap-3 mt-1">
                            <span className="text-xs text-zinc-500">
                              {formatTime(msg.timestamp)}{msg.responseTimeMs !== undefined ? ` • ${formatResponseTime(msg.responseTimeMs)}` : ''}
                            </span>
                            <MessageStars msgId={msg.id} current={messageRatings[msg.id] ?? 0} onRate={handleMessageRating} />
                          </div>
                        </div>
                      </div>
                    </div>
                  )
                })()}
                {msg.type === 'user' && (
                  <div className="self-end max-w-[80%] ml-auto">
                    <div className={`rounded-2xl px-4 py-2 ${msg.piiMasked ? 'bg-orange-200 text-orange-900' : 'bg-orange-600 text-white'}`}>
                      <div className={`markdown-content ${msg.piiMasked ? 'text-orange-900' : 'text-white'}`}>
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-400 text-right flex items-center justify-end gap-1">
                      {msg.piiMasked && <span title="Персональные данные скрыты (152-ФЗ)">🔒</span>}
                      {formatTime(msg.timestamp)}
                    </div>
                  </div>
                )}
                {msg.type === 'system' && (
                  <div className="self-start max-w-[80%]">
                    <div className="rounded-2xl px-4 py-2 bg-zinc-100 text-zinc-900 border border-zinc-200">
                      <div className="text-xs uppercase tracking-wide text-zinc-500 mb-1">{sessionMode === 'study' ? 'Система' : 'Интервьюер'}</div>
                      <div className="markdown-content">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-500">
                      {formatTime(msg.timestamp)}
                    </div>
                  </div>
                )}
                {msg.type === 'hint' && (
                  <div className="self-start max-w-[80%]">
                    <div className="flex items-start gap-2">
                      <div className="w-7 h-7 rounded-full bg-yellow-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                        <span className="text-yellow-600 text-xs">💡</span>
                      </div>
                      <div>
                        <div className="rounded-2xl px-4 py-2 bg-yellow-50 text-yellow-900 border border-yellow-200">
                          <div className="text-xs uppercase tracking-wide text-yellow-600 mb-1">Подсказка</div>
                          <div className="markdown-content">
                            <ReactMarkdown>{msg.content}</ReactMarkdown>
                          </div>
                        </div>
                        <div className="flex items-center gap-3 mt-1">
                          <span className="text-xs text-zinc-500">{formatTime(msg.timestamp)}</span>
                          <MessageStars msgId={msg.id} current={messageRatings[msg.id] ?? 0} onRate={handleMessageRating} />
                        </div>
                      </div>
                    </div>
                  </div>
                )}
                {msg.type === 'summary' && (
                  <div className="self-start max-w-[90%]">
                    <div className="flex items-start gap-2">
                      <div className="w-7 h-7 rounded-full bg-blue-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                        <span className="text-blue-600 text-xs font-bold">AI</span>
                      </div>
                      <div>
                        <div className="rounded-2xl px-4 py-3 bg-blue-50 text-blue-900 border border-blue-200">
                          <div className="text-xs uppercase tracking-wide text-blue-600 mb-1">Промежуточная сводка</div>
                          <div className="markdown-content">
                            <ReactMarkdown>{msg.content}</ReactMarkdown>
                          </div>
                        </div>
                        <div className="flex items-center gap-3 mt-1">
                          <span className="text-xs text-zinc-500">{formatTime(msg.timestamp)}</span>
                          <MessageStars msgId={msg.id} current={messageRatings[msg.id] ?? 0} onRate={handleMessageRating} />
                        </div>
                      </div>
                    </div>
                  </div>
                )}
                {msg.type === 'answer' && (
                  <div className="self-start max-w-[80%]">
                    <div className="rounded-2xl px-4 py-2 bg-white border border-orange-100 shadow-soft">
                      <div className="markdown-content">
                        <ReactMarkdown>{`Ответ: ${msg.content}`}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-500">
                      {formatTime(msg.timestamp)}{msg.responseTimeMs !== undefined ? ` • ответ за ${formatResponseTime(msg.responseTimeMs)}` : ''}
                    </div>
                  </div>
                )}
              </div>
            ))}
            {isLoading && !chatCompleted && (
              <div className="flex items-start gap-2 py-1">
                <div className="w-7 h-7 rounded-full bg-orange-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <span className="text-orange-600 text-xs font-bold">AI</span>
                </div>
                <div className="rounded-2xl px-4 py-2.5 bg-zinc-50 border border-zinc-200 text-sm text-zinc-500 flex items-center gap-2">
                  <span className="flex gap-0.5">
                    <span className="w-1.5 h-1.5 bg-zinc-400 rounded-full animate-bounce" style={{animationDelay: '0ms'}} />
                    <span className="w-1.5 h-1.5 bg-zinc-400 rounded-full animate-bounce" style={{animationDelay: '150ms'}} />
                    <span className="w-1.5 h-1.5 bg-zinc-400 rounded-full animate-bounce" style={{animationDelay: '300ms'}} />
                  </span>
                  {stepLabel ? (
                    <span className="text-xs text-zinc-400 italic">{stepLabel}…</span>
                  ) : (
                    <span className="text-xs text-zinc-400 italic">Агент обрабатывает ответ…</span>
                  )}
                </div>
              </div>
            )}
            <div ref={bottomRef} />
          </div>
        </div>
        {(() => {
          const lastQMsg = [...messages].reverse().find(m => m.type === 'question')
          const planMsg = sessionMode === 'study' ? messages.find(m => m.type === 'question' && parseStudyContent(m.content).type === 'plan') : null
          const planUnrevealed = planMsg != null && !revealedQuestions.has(planMsg.id)
          const studyBlocked = sessionMode === 'study' && (
            planUnrevealed ||
            (lastQMsg != null && !revealedQuestions.has(lastQMsg.id) && parseStudyContent(lastQMsg.content).type === 'theory_question')
          )
          return (
            <div className="flex flex-col gap-2">
              <textarea
                rows={3}
                className="w-full px-3 py-2 rounded-lg border border-orange-200 focus:outline-none focus:ring-2 focus:ring-orange-500 resize-none overflow-y-auto whitespace-pre-wrap"
                placeholder={studyBlocked ? 'Нажмите «Продолжить», чтобы ответить...' : 'Напишите ответ'}
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                onKeyDown={handleKeyDown}
                disabled={isLoading || !sessionId || chatCompleted || studyBlocked}
              />
              <div className="flex flex-col gap-2 w-40 mx-auto">
                <button
                  onClick={handleSend}
                  disabled={isLoading || !sessionId || !inputValue.trim() || chatCompleted || studyBlocked}
                  className="w-40 px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700 disabled:bg-orange-300 disabled:cursor-not-allowed"
                >
                  Отправить
                </button>
                {sessionMode !== 'study' && (
                  <button
                    onClick={handleTerminate}
                    disabled={!sessionId || chatCompleted}
                    className="w-40 px-4 py-2 rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:bg-red-300 disabled:cursor-not-allowed"
                    title={sessionMode === 'training' ? 'Завершить тренировку' : 'Досрочно завершить интервью'}
                  >
                    Завершить
                  </button>
                )}
              </div>
            </div>
          )
        })()}
      </main>
    </div>
  )
}
