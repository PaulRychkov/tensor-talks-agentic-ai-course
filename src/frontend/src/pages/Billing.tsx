/**
 * Billing page stub (§10.2).
 *
 * MVP: service is free. Shows informational page.
 * When billingEnabled=true (future), shows subscription + payments.
 */

import { useNavigate } from 'react-router-dom'

const BILLING_ENABLED = import.meta.env.VITE_BILLING_ENABLED === 'true'

export default function Billing() {
  const navigate = useNavigate()

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
          <h1 className="text-2xl font-bold text-gray-800">Подписка и оплата</h1>
        </div>

        {!BILLING_ENABLED ? (
          <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-8 text-center">
            <div className="text-5xl mb-4">🎁</div>
            <h2 className="text-xl font-semibold text-gray-800 mb-2">
              Сервис бесплатен на этапе MVP
            </h2>
            <p className="text-gray-500 text-sm leading-relaxed">
              TensorTalks находится в стадии открытого бета-тестирования. Все функции
              доступны бесплатно. Монетизация будет введена позже — с заблаговременным
              уведомлением всех пользователей.
            </p>
            <div className="mt-6 p-4 bg-orange-50 rounded-lg text-left space-y-2">
              <p className="text-sm font-medium text-orange-800">Что входит в бесплатный доступ:</p>
              <ul className="text-sm text-orange-700 space-y-1">
                <li>✓ Неограниченные интервью</li>
                <li>✓ Все режимы: интервью, тренировка, изучение</li>
                <li>✓ AI-аналитика и отчёты</li>
                <li>✓ История сессий и прогресс</li>
              </ul>
            </div>
          </div>
        ) : (
          <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-5">
            <p className="text-gray-500">Загрузка данных подписки...</p>
          </div>
        )}
      </div>
    </div>
  )
}
