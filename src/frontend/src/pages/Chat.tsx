import { Link, useParams, useNavigate } from 'react-router-dom'
import { useState, useEffect, useRef, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import { sendMessage, getNextQuestion, getResults, resumeChat, getInterviewChat, terminateChat, type ResultsResponse, type ChatMessage } from '../services/chat'
import TerminateConfirmModal from '../components/TerminateConfirmModal'

interface Message {
  id: string
  type: 'question' | 'answer' | 'user' | 'system'
  content: string
  timestamp: string
  responseTimeMs?: number
}

export default function Chat() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [messages, setMessages] = useState<Message[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [sessionId] = useState<string | null>(id || null)
  const [userId, setUserId] = useState<string | null>(null)
  const [chatCompleted, setChatCompleted] = useState(false)
  const [results, setResults] = useState<ResultsResponse | null>(null)
  const [showTerminateModal, setShowTerminateModal] = useState(false)
  const [waitingSince, setWaitingSince] = useState<number | null>(null)
  const [waitingSeconds, setWaitingSeconds] = useState(0)
  const pollingIntervalRef = useRef<number | null>(null)
  const waitingSinceRef = useRef<number | null>(null)
  const hasResumedRef = useRef(false)
  const chatContainerRef = useRef<HTMLDivElement | null>(null)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const resultsRef = useRef<HTMLDivElement | null>(null)
  const resultsScrollTimeoutRef = useRef<number | null>(null)

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
          setChatCompleted(true)
          setResults(chatResults)
          setIsLoading(false)
          setWaitingSince(null)
          if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current)
          }
          try {
            const chatHistory = await getInterviewChat(currentSessionId)
            if (chatHistory && chatHistory.length > 0) {
              setMessages(mapHistoryMessages(chatHistory))
            }
          } catch (error) {
            console.warn('Failed to load chat history after completion:', error)
          }
          return
        }

        // Получаем следующий вопрос
        const question = await getNextQuestion(currentSessionId)
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
            // Проверяем, нет ли уже такого вопроса
            if (prev.some(m => m.id === question.question_id)) {
              return prev
            }
            return [...prev, questionMessage]
          })
          setIsLoading(false)
          setWaitingSince(null)
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
      if (msg.type === 'system') {
        const content = msg.content || ''
        if (content.startsWith('Вопрос: ')) {
          return {
            id: `history-question-${idx}`,
            type: 'question' as const,
            content: content.substring('Вопрос: '.length),
            timestamp: createdAt,
          }
        }
        return {
          id: `history-system-${idx}`,
          type: 'system' as const,
          content,
          timestamp: createdAt,
        }
      }
      if (msg.type === 'user') {
        return {
          id: `history-user-${idx}`,
          type: 'user' as const,
          content: msg.content,
          timestamp: createdAt,
        }
      }
      return {
        id: `history-question-${idx}`,
        type: 'question' as const,
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
              }
            } catch (error) {
              console.warn('Failed to load chat history, continuing without it:', error)
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

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [messages])

  useEffect(() => {
    if (results && chatCompleted) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
      if (resultsScrollTimeoutRef.current) {
        clearTimeout(resultsScrollTimeoutRef.current)
      }
      resultsScrollTimeoutRef.current = window.setTimeout(() => {
        resultsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 5000)
    }
    return () => {
      if (resultsScrollTimeoutRef.current) {
        clearTimeout(resultsScrollTimeoutRef.current)
      }
    }
  }, [results, chatCompleted])

  const startNewChat = () => {
    // Если нет сессии, перенаправляем на дашборд для выбора параметров
    navigate('/dashboard')
  }

  const handleSend = async () => {
    if (!inputValue.trim() || !sessionId || !userId || isLoading || chatCompleted) return

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
    } catch (error) {
      console.error('Failed to send message:', error)
      alert('Не удалось отправить сообщение. Попробуйте еще раз.')
      setIsLoading(false)
      setWaitingSince(null)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
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

      // Останавливаем polling и устанавливаем флаг завершения
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current)
      }
      setChatCompleted(true)
      setIsLoading(false)
      setWaitingSince(null)
      
      // Получаем результаты через небольшую задержку
      setTimeout(async () => {
        try {
          const chatResults = await getResults(sessionId)
          if (chatResults) {
            setResults(chatResults)
            try {
              const chatHistory = await getInterviewChat(sessionId)
              if (chatHistory && chatHistory.length > 0) {
                setMessages(mapHistoryMessages(chatHistory))
              }
            } catch (error) {
              console.warn('Failed to load chat history after termination:', error)
            }
          }
        } catch (error) {
          console.error('Failed to get results after termination:', error)
        }
      }, 1000)
    } catch (error) {
      console.error('Failed to terminate chat:', error)
      // Не закрываем модальное окно при ошибке - пользователь может попробовать снова
      // Ошибка уже логируется в консоли, пользователь увидит проблему через UI
    }
  }

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
      />
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <Link to="/dashboard" className="text-sm text-orange-600 hover:underline">
            ← К дашборду
          </Link>
          <div className="font-semibold">
            {sessionId ? `Сессия: ${sessionId.substring(0, 8)}...` : 'Создание сессии...'}
          </div>
        </div>
      </header>
      <main className="max-w-4xl mx-auto px-4 py-8 flex flex-col gap-4 h-[90vh]">
        <div className="bg-white rounded-xl border border-orange-100 p-4 flex-1 overflow-y-auto" ref={chatContainerRef}>
          <div className="text-sm text-zinc-500 mb-2">Чат с AI‑интервьюером</div>
          {waitingSince && (
            <div className="text-xs text-zinc-500 mb-2">
              Ожидание ответа: {waitingSeconds}с
            </div>
          )}
          <div className="space-y-4">
            {messages.length === 0 && !isLoading && !chatCompleted && (
              <div className="text-center text-zinc-400 py-8">
                Чат начат. Ожидайте первого вопроса от AI-интервьюера...
              </div>
            )}
            {chatCompleted && results && (
              <div ref={resultsRef} className="bg-gradient-to-r from-orange-50 to-rose-50 rounded-xl p-6 border border-orange-200">
                <h3 className="text-xl font-semibold mb-4 text-orange-900">Интервью завершено!</h3>
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
                  {results.recommendations.length > 0 && (
                    <div>
                      <div className="text-sm text-zinc-600 mb-2">Рекомендации</div>
                      <ul className="list-disc list-inside space-y-1 text-zinc-800">
                        {results.recommendations.map((rec, idx) => (
                          <li key={idx}>{rec}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
                <button
                  onClick={() => navigate('/dashboard')}
                  className="mt-4 px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700"
                >
                  Вернуться к дашборду
                </button>
              </div>
            )}
            {messages.map((msg) => (
              <div key={msg.id} className="grid gap-2">
                {msg.type === 'question' && (
                  <div className="self-start max-w-[80%]">
                    <div className="rounded-2xl px-4 py-2 bg-orange-100 text-zinc-900">
                      <div className="markdown-content">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-500">
                      {formatTime(msg.timestamp)}{msg.responseTimeMs !== undefined ? ` • ответ за ${formatResponseTime(msg.responseTimeMs)}` : ''}
                    </div>
                  </div>
                )}
                {msg.type === 'user' && (
                  <div className="self-end max-w-[80%] ml-auto">
                    <div className="rounded-2xl px-4 py-2 bg-orange-600 text-white">
                      <div className="markdown-content text-white">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-400 text-right">
                      {formatTime(msg.timestamp)}
                    </div>
                  </div>
                )}
                {msg.type === 'system' && (
                  <div className="self-start max-w-[80%]">
                    <div className="rounded-2xl px-4 py-2 bg-zinc-100 text-zinc-900 border border-zinc-200">
                      <div className="text-xs uppercase tracking-wide text-zinc-500 mb-1">Интервьюер</div>
                      <div className="markdown-content">
                        <ReactMarkdown>{msg.content}</ReactMarkdown>
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-zinc-500">
                      {formatTime(msg.timestamp)}
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
              <div className="text-center text-zinc-400 py-2">
                <div className="inline-block animate-pulse">Обработка...</div>
              </div>
            )}
            <div ref={bottomRef} />
          </div>
        </div>
        <div className="flex gap-2">
          <input 
            className="flex-1 px-3 py-2 rounded-lg border border-orange-200 focus:outline-none focus:ring-2 focus:ring-orange-500"
            placeholder="Напишите ответ..." 
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={isLoading || !sessionId || chatCompleted}
          />
          <button
            onClick={handleSend}
            disabled={isLoading || !sessionId || !inputValue.trim() || chatCompleted}
            className="px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700 disabled:bg-orange-300 disabled:cursor-not-allowed"
          >
            Отправить
          </button>
          <button
            onClick={handleTerminate}
            disabled={!sessionId || chatCompleted}
            className="px-4 py-2 rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:bg-red-300 disabled:cursor-not-allowed"
            title="Досрочно завершить интервью"
          >
            Завершить
          </button>
        </div>
      </main>
    </div>
  )
}
