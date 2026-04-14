/**
 * Account recovery page — reset password using a recovery key (§10.10).
 * Route: /auth/recover
 */

import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

const apiBaseEnv = import.meta.env.VITE_API_BASE_URL ?? '/api'
const API_BASE = apiBaseEnv.replace(/\/$/, '')

export default function Recover() {
  const navigate = useNavigate()
  const [login, setLogin] = useState('')
  const [recoveryKey, setRecoveryKey] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newPasswordRepeat, setNewPasswordRepeat] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [success, setSuccess] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!login.trim() || !recoveryKey.trim() || !newPassword || !newPasswordRepeat) {
      setError('Заполните все поля')
      return
    }
    if (newPassword !== newPasswordRepeat) {
      setError('Пароли не совпадают')
      return
    }

    setIsSubmitting(true)
    try {
      const res = await fetch(`${API_BASE}/auth/recover`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login: login.trim(),
          recovery_key: recoveryKey.trim(),
          new_password: newPassword,
        }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => null)
        throw new Error(data?.error ?? `Ошибка: ${res.status}`)
      }
      setSuccess(true)
      setTimeout(() => navigate('/auth'), 3000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Произошла ошибка')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-orange-50 to-amber-50 flex items-center justify-center px-4">
      <div className="bg-white rounded-2xl shadow-lg p-8 w-full max-w-md">
        <div className="text-center mb-6">
          <div className="text-4xl mb-2">🔑</div>
          <h1 className="text-2xl font-bold text-gray-800">Восстановление доступа</h1>
          <p className="text-sm text-gray-500 mt-1">
            Введите логин и ключ восстановления
          </p>
        </div>

        {success ? (
          <div className="text-center">
            <div className="text-4xl mb-3">✓</div>
            <p className="text-green-700 font-medium">Пароль успешно изменён</p>
            <p className="text-sm text-gray-500 mt-1">Перенаправление на страницу входа...</p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Логин</label>
              <input
                type="text"
                value={login}
                onChange={e => setLogin(e.target.value)}
                placeholder="BrightNeural42"
                className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Ключ восстановления</label>
              <input
                type="text"
                value={recoveryKey}
                onChange={e => setRecoveryKey(e.target.value)}
                placeholder="ABCD-EFGH-IJKL-MNOP-..."
                className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-orange-300"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Новый пароль</label>
              <input
                type="password"
                value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Повторите новый пароль</label>
              <input
                type="password"
                value={newPasswordRepeat}
                onChange={e => setNewPasswordRepeat(e.target.value)}
                className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
              />
            </div>
            {error && <p className="text-red-600 text-sm">{error}</p>}
            <button
              type="submit"
              disabled={isSubmitting}
              className="w-full bg-orange-500 hover:bg-orange-600 disabled:bg-orange-300 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
            >
              {isSubmitting ? 'Восстановление...' : 'Восстановить доступ'}
            </button>
          </form>
        )}

        <div className="text-center mt-4">
          <Link to="/auth" className="text-sm text-orange-500 hover:underline">
            ← Вернуться к входу
          </Link>
        </div>
      </div>
    </div>
  )
}
