/**
 * Account management page (§10.2).
 *
 * Sections:
 *  - Profile (login read-only, UUID)
 *  - Change password
 *  - Regenerate recovery key
 *  - Logout
 *  - Delete account
 */

import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

const apiBaseEnv = import.meta.env.VITE_API_BASE_URL ?? '/api'
const API_BASE = apiBaseEnv.replace(/\/$/, '')

function getAuthHeaders(): HeadersInit {
  try {
    const tokens = JSON.parse(localStorage.getItem('tt_tokens') ?? '{}')
    const accessToken = tokens?.access_token
    if (accessToken) return { Authorization: `Bearer ${accessToken}` }
  } catch {}
  return {}
}

async function authFetch(path: string, options: RequestInit = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
      ...(options.headers ?? {}),
    },
  })
  if (!res.ok) {
    const data = await res.json().catch(() => null)
    throw new Error(data?.error ?? `Request failed: ${res.status}`)
  }
  return res.json()
}

type UserInfo = {
  id: string
  login: string
}

export default function Account() {
  const navigate = useNavigate()
  const [user, setUser] = useState<UserInfo | null>(null)

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newPasswordRepeat, setNewPasswordRepeat] = useState('')
  const [changePwError, setChangePwError] = useState<string | null>(null)
  const [changePwSuccess, setChangePwSuccess] = useState(false)
  const [isChangingPw, setIsChangingPw] = useState(false)

  const [recoveryKey, setRecoveryKey] = useState<string | null>(null)
  const [isGeneratingKey, setIsGeneratingKey] = useState(false)

  const [isDeleting, setIsDeleting] = useState(false)
  const [deletePassword, setDeletePassword] = useState('')
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  useEffect(() => {
    try {
      const raw = localStorage.getItem('tt_user')
      if (raw) setUser(JSON.parse(raw))
      else navigate('/auth', { replace: true })
    } catch {
      navigate('/auth', { replace: true })
    }
  }, [navigate])

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    setChangePwError(null)
    setChangePwSuccess(false)
    if (newPassword !== newPasswordRepeat) {
      setChangePwError('Пароли не совпадают')
      return
    }
    if (newPassword.length === 0) {
      setChangePwError('Новый пароль не может быть пустым')
      return
    }
    setIsChangingPw(true)
    try {
      await authFetch('/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      })
      setChangePwSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setNewPasswordRepeat('')
    } catch (err) {
      setChangePwError(err instanceof Error ? err.message : 'Ошибка при смене пароля')
    } finally {
      setIsChangingPw(false)
    }
  }

  const handleRegenerateRecoveryKey = async () => {
    if (!currentPassword) {
      alert('Введите текущий пароль для подтверждения')
      return
    }
    setIsGeneratingKey(true)
    try {
      const result = await authFetch('/auth/regenerate-recovery-key', {
        method: 'POST',
        body: JSON.stringify({ password: currentPassword }),
      })
      setRecoveryKey(result.recovery_key)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Ошибка при генерации ключа восстановления')
    } finally {
      setIsGeneratingKey(false)
    }
  }

  const handleLogout = async () => {
    try {
      await authFetch('/auth/logout', { method: 'POST' })
    } catch {}
    localStorage.removeItem('tt_tokens')
    localStorage.removeItem('tt_user')
    navigate('/auth', { replace: true })
  }

  const handleDeleteAccount = async () => {
    setDeleteError(null)
    setIsDeleting(true)
    try {
      await authFetch('/auth/account', {
        method: 'DELETE',
        body: JSON.stringify({ password: deletePassword }),
      })
      localStorage.removeItem('tt_tokens')
      localStorage.removeItem('tt_user')
      navigate('/auth', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Ошибка при удалении аккаунта')
    } finally {
      setIsDeleting(false)
    }
  }

  if (!user) return null

  return (
    <div className="min-h-screen bg-gray-50 py-8 px-4">
      <div className="max-w-lg mx-auto space-y-6">
        <div className="flex items-center gap-3 mb-4">
          <button
            onClick={() => navigate('/dashboard')}
            className="text-orange-500 hover:text-orange-700 text-sm font-medium"
          >
            ← Назад
          </button>
          <h1 className="text-2xl font-bold text-gray-800">Учётная запись</h1>
        </div>

        {/* Profile section */}
        <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-5">
          <h2 className="text-lg font-semibold text-gray-700 mb-3">Профиль</h2>
          <div className="space-y-2 text-sm text-gray-600">
            <div>
              <span className="font-medium text-gray-700">Логин:</span>{' '}
              <span className="font-mono bg-gray-100 px-2 py-0.5 rounded">{user.login}</span>
            </div>
            <div>
              <span className="font-medium text-gray-700">UUID:</span>{' '}
              <span className="font-mono text-xs text-gray-500">{user.id}</span>
            </div>
          </div>
        </div>

        {/* Change password */}
        <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-5">
          <h2 className="text-lg font-semibold text-gray-700 mb-3">Смена пароля</h2>
          <form onSubmit={handleChangePassword} className="space-y-3">
            <input
              type="password"
              placeholder="Текущий пароль"
              value={currentPassword}
              onChange={e => setCurrentPassword(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
            />
            <input
              type="password"
              placeholder="Новый пароль"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
            />
            <input
              type="password"
              placeholder="Повторите новый пароль"
              value={newPasswordRepeat}
              onChange={e => setNewPasswordRepeat(e.target.value)}
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-orange-300"
            />
            {changePwError && (
              <p className="text-red-600 text-sm">{changePwError}</p>
            )}
            {changePwSuccess && (
              <p className="text-green-600 text-sm">Пароль успешно изменён</p>
            )}
            <button
              type="submit"
              disabled={isChangingPw}
              className="w-full bg-orange-500 hover:bg-orange-600 disabled:bg-orange-300 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
            >
              {isChangingPw ? 'Сохранение...' : 'Изменить пароль'}
            </button>
          </form>
        </div>

        {/* Recovery key */}
        <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-5">
          <h2 className="text-lg font-semibold text-gray-700 mb-1">Ключ восстановления</h2>
          <p className="text-sm text-gray-500 mb-3">
            Используется для восстановления доступа при утере пароля.
            После перегенерации старый ключ станет недействительным.
          </p>
          {recoveryKey ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2 bg-gray-50 border border-gray-200 rounded-lg px-3 py-2">
                <span className="font-mono text-sm text-gray-800 flex-1 break-all">
                  {recoveryKey}
                </span>
                <button
                  onClick={() => navigator.clipboard.writeText(recoveryKey)}
                  className="text-orange-500 hover:text-orange-700 text-xs whitespace-nowrap"
                >
                  📋 Копировать
                </button>
              </div>
              <p className="text-xs text-red-600">
                ⚠ Сохраните ключ. Он показывается один раз и не может быть восстановлен.
              </p>
            </div>
          ) : (
            <button
              onClick={handleRegenerateRecoveryKey}
              disabled={isGeneratingKey}
              className="bg-gray-100 hover:bg-gray-200 disabled:opacity-50 text-gray-700 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
            >
              {isGeneratingKey ? 'Генерация...' : 'Перегенерировать ключ восстановления'}
            </button>
          )}
        </div>

        {/* Logout */}
        <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-5">
          <h2 className="text-lg font-semibold text-gray-700 mb-3">Выход</h2>
          <button
            onClick={handleLogout}
            className="w-full border border-orange-400 text-orange-500 hover:bg-orange-50 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
          >
            Выйти из аккаунта
          </button>
        </div>

        {/* Delete account */}
        <div className="bg-white rounded-xl border border-red-100 shadow-sm p-5">
          <h2 className="text-lg font-semibold text-red-700 mb-1">Удаление аккаунта</h2>
          <p className="text-sm text-gray-500 mb-3">
            Это действие необратимо. Все данные будут удалены.
          </p>
          {!showDeleteConfirm ? (
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="border border-red-300 text-red-600 hover:bg-red-50 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
            >
              Удалить аккаунт
            </button>
          ) : (
            <div className="space-y-3">
              <input
                type="password"
                placeholder="Введите пароль для подтверждения"
                value={deletePassword}
                onChange={e => setDeletePassword(e.target.value)}
                className="w-full border border-red-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-red-300"
              />
              {deleteError && <p className="text-red-600 text-sm">{deleteError}</p>}
              <div className="flex gap-2">
                <button
                  onClick={handleDeleteAccount}
                  disabled={isDeleting}
                  className="flex-1 bg-red-600 hover:bg-red-700 disabled:opacity-50 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
                >
                  {isDeleting ? 'Удаление...' : 'Подтвердить удаление'}
                </button>
                <button
                  onClick={() => { setShowDeleteConfirm(false); setDeletePassword(''); setDeleteError(null) }}
                  className="flex-1 border border-gray-300 text-gray-600 hover:bg-gray-50 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
                >
                  Отмена
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
