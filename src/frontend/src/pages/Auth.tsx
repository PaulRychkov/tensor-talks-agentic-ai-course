import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { login as loginRequest, register as registerRequest } from '../services/auth'

export default function Auth() {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const navigate = useNavigate()
  const [isLoaded, setIsLoaded] = useState(false)
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [passwordRepeat, setPasswordRepeat] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    const timer = setTimeout(() => setIsLoaded(true), 100)
    return () => clearTimeout(timer)
  }, [])

  const validate = (): boolean => {
    setError(null)
    const trimmedLogin = login.trim()
    if (trimmedLogin.length < 3 || trimmedLogin.length > 30) {
      setError('Логин должен быть от 3 до 30 символов')
      return false
    }
    const allowed = /^[a-zA-Z0-9._-]+$/
    if (!allowed.test(trimmedLogin)) {
      setError('Логин может содержать только латинские буквы, цифры, . _ -')
      return false
    }
    if (trimmedLogin.includes('@')) {
      setError('Логин не должен быть email или содержать персональные данные')
      return false
    }
    if (/\d{6,}/.test(trimmedLogin)) {
      setError('Логин не должен содержать номера телефонов или персональные данные')
      return false
    }
    // if (password.length < 8) {
    //   setError('Пароль должен быть не короче 8 символов')
    //   return false
    // }
    // // Проверка наличия букв и цифр в пароле
    // const hasLetter = /[a-zA-Z]/.test(password)
    // const hasDigit = /[0-9]/.test(password)
    // if (!hasLetter || !hasDigit) {
    //   setError('Пароль должен содержать хотя бы одну букву и одну цифру')
    //   return false
    // }
    if (password.length === 0) {
      setError('Пароль не может быть пустым')
      return false
    }
    if (mode === 'register' && password !== passwordRepeat) {
      setError('Пароли не совпадают')
      return false
    }
    return true
  }

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate()) return

    setIsSubmitting(true)
    setError(null)
    try {
      const action = mode === 'register' ? registerRequest : loginRequest
      const response = await action(login.trim(), password)
      localStorage.setItem('tt_user', JSON.stringify(response.user))
      localStorage.setItem('tt_tokens', JSON.stringify(response.tokens))
      navigate('/dashboard')
    } catch (err) {
      if (err instanceof Error) {
        setError(err.message)
      } else {
        setError('Не удалось выполнить запрос. Попробуйте позже.')
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-orange-50 via-white to-rose-50 relative overflow-hidden">
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-40 -right-40 w-80 h-80 bg-gradient-to-tr from-orange-300/20 to-rose-300/20 rounded-full blur-3xl animate-pulse" />
        <div className="absolute -bottom-40 -left-40 w-80 h-80 bg-gradient-to-tr from-blue-300/20 to-purple-300/20 rounded-full blur-3xl" />
        <div className="absolute top-1/2 left-1/4 w-40 h-40 bg-gradient-to-tr from-green-300/20 to-blue-300/20 rounded-full blur-2xl animate-bounce" />
        <div className="absolute top-1/4 right-1/3 w-32 h-32 bg-gradient-to-tr from-purple-300/20 to-pink-300/20 rounded-full blur-2xl animate-pulse" />
      </div>

      <div className="relative z-10 flex items-center justify-center min-h-screen p-6">
        <div className={`w-full max-w-xl bg-white/80 backdrop-blur-sm rounded-3xl border border-orange-100 shadow-2xl p-6 transition-all duration-1000 transform ${isLoaded ? 'translate-y-0 opacity-100' : 'translate-y-8 opacity-0'}`}>
          <div className="flex items-center justify-between mb-3">
            <Link to="/" className="text-sm text-orange-600 hover:text-orange-700 hover:underline transition-colors duration-200 flex items-center gap-1">
              <span>←</span> На главную
            </Link>
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded-lg bg-gradient-to-br from-orange-500 to-rose-500" />
              <span className="text-sm font-semibold text-zinc-700">TensorTalks</span>
            </div>
          </div>

          <div className="text-center mb-3">
            <div className="mx-auto size-12 rounded-2xl bg-gradient-to-tr from-orange-500 to-rose-500 shadow-lg flex items-center justify-center mb-2" style={{animation: 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite'}}>
              <span className="text-white text-2xl">🧠</span>
            </div>
            <h1 className="text-xl font-bold text-zinc-900 mb-1">
              {mode === 'login' ? 'Добро пожаловать!' : 'Присоединяйтесь к нам'}
            </h1>
            <p className="text-sm text-zinc-500 mb-2">
              AI‑симулятор технических ML‑собеседований
            </p>
            <div className="bg-gradient-to-r from-orange-50 to-rose-50 rounded-xl p-1.5 border border-orange-100">
              <div className="flex items-center justify-center gap-6 text-xs text-zinc-600">
                <div className="flex items-center gap-1">
                  <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></span>
                  Объективная оценка
                </div>
                <div className="flex items-center gap-1">
                  <span className="w-2 h-2 bg-blue-500 rounded-full animate-pulse"></span>
                  AI-интервьюер
                </div>
                <div className="flex items-center gap-1">
                  <span className="w-2 h-2 bg-purple-500 rounded-full animate-pulse"></span>
                  Персональные рекомендации
                </div>
              </div>
            </div>
          </div>

          <form className="grid gap-2.5" onSubmit={onSubmit}>
            <label className="grid gap-2">
              <span className="text-sm font-medium text-zinc-700 pl-4">Логин</span>
              <input
                type="text"
                className="px-4 py-3 rounded-xl border border-zinc-200 bg-white hover:border-orange-300 focus:border-orange-500 focus:ring-2 focus:ring-orange-100 transition-all duration-200"
                placeholder="например: ml.interviewee_2025"
                value={login}
                onChange={(e) => setLogin(e.target.value)}
              />
              {mode === 'register' && (
                <span className="text-xs text-zinc-500 pl-4">
                  Логин не должен содержать персональные данные (ФИО, email, телефон)
                </span>
              )}
            </label>
            <label className="grid gap-2">
              <span className="text-sm font-medium text-zinc-700 pl-4">Пароль</span>
              <input
                type="password"
                className="px-4 py-3 rounded-xl border border-zinc-200 bg-white hover:border-orange-300 focus:border-orange-500 focus:ring-2 focus:ring-orange-100 transition-all duration-200"
                placeholder="••••••••••"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            {mode === 'register' && (
              <label className="grid gap-2">
                <span className="text-sm font-medium text-zinc-700 pl-4">Повторите пароль</span>
                <input
                  type="password"
                  className="px-4 py-3 rounded-xl border border-zinc-200 bg-white hover:border-orange-300 focus:border-orange-500 focus:ring-2 focus:ring-orange-100 transition-all duration-200"
                  placeholder="••••••••••"
                  value={passwordRepeat}
                  onChange={(e) => setPasswordRepeat(e.target.value)}
                />
              </label>
            )}

            {mode === 'register' && (
              <div className="rounded-xl border border-orange-100 bg-gradient-to-r from-orange-50 to-rose-50 p-3">
                <p className="text-xs text-zinc-600">
                  Мы не используем cookies, не обрабатываем и не храним персональные данные пользователей.
                  Логин должен быть обезличенным и не содержать персональные данные. Подробнее см.{' '}
                  <Link to="/privacy-policy" className="text-orange-600 hover:underline">Политику конфиденциальности</Link>.
                </p>
              </div>
            )}

            {error && (
              <div className="rounded-lg border border-rose-200 bg-rose-50 text-rose-700 text-sm px-3 py-2">
                {error}
              </div>
            )}

            <button
              className="px-6 py-3 rounded-xl bg-gradient-to-r from-orange-600 to-rose-600 text-white font-semibold hover:from-orange-700 hover:to-rose-700 shadow-lg hover:shadow-xl transform hover:scale-102 transition-all duration-300 disabled:opacity-60 disabled:cursor-not-allowed"
              disabled={isSubmitting}
            >
              {isSubmitting
                ? 'Обработка...'
                : mode === 'login'
                  ? 'Войти в систему'
                  : 'Создать аккаунт'}
            </button>
          </form>

          <div className="mt-3 text-center">
            <div className="flex items-center gap-2 justify-center text-xs text-zinc-500">
              <div className="h-px bg-zinc-200 flex-1" />
              <span className="shrink-0">или</span>
              <div className="h-px bg-zinc-200 flex-1" />
            </div>
            <div className="mt-4 text-sm text-zinc-600">
              {mode === 'login' ? (
                <button
                  className="text-orange-600 hover:text-orange-700 hover:underline font-medium transition-colors duration-200"
                  onClick={() => setMode('register')}
                >
                  Нет аккаунта? Зарегистрироваться
                </button>
              ) : (
                <button
                  className="text-orange-600 hover:text-orange-700 hover:underline font-medium transition-colors duration-200"
                  onClick={() => setMode('login')}
                >
                  Уже есть аккаунт? Войти
                </button>
              )}
            </div>
          </div>

          <div className="mt-3 pt-2 border-t border-zinc-100">
            <div className="text-center">
              <p className="text-xs text-zinc-500 mb-3">Что вас ждет после регистрации:</p>
              <div className="flex justify-center">
                <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-xs text-zinc-600 max-w-md">
                  <div className="flex items-start gap-2">
                    <span className="text-green-500 flex-shrink-0 mt-0.5">✓</span>
                    <span className="leading-tight">Доступ к базе вопросов</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-green-500 flex-shrink-0 mt-0.5">✓</span>
                    <span className="leading-tight">AI-разбор ваших ответов</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-green-500 flex-shrink-0 mt-0.5">✓</span>
                    <span className="leading-tight">Персональные рекомендации</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-green-500 flex-shrink-0 mt-0.5">✓</span>
                    <span className="leading-tight">Отслеживание прогресса</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

