import { Link } from 'react-router-dom'

export default function Privacy() {
  return (
    <div className="min-h-screen bg-gradient-to-br from-orange-50 via-white to-rose-50">
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur">
        <div className="max-w-5xl mx-auto px-4 py-4 flex items-center justify-between">
          <Link to="/" className="text-sm text-orange-600 hover:underline">← На главную</Link>
          <div className="flex items-center gap-2">
            <div className="w-6 h-6 rounded-lg bg-gradient-to-br from-orange-500 to-rose-500" />
            <span className="text-sm font-semibold text-zinc-700">TensorTalks</span>
          </div>
        </div>
      </header>

      <main className="max-w-3xl mx-auto px-4 py-8">
        <div className="bg-white rounded-3xl border border-orange-100 shadow-xl p-6">
          <h1 className="text-2xl font-bold text-zinc-900 mb-3">Политика конфиденциальности</h1>
          <p className="text-sm text-zinc-500 mb-6">Обновлено: 2025‑11‑12</p>

          <div className="space-y-5 text-zinc-700 leading-relaxed">
            <section>
              <h2 className="font-semibold text-zinc-900 mb-2">1. Общие положения</h2>
              <p>
                Настоящая политика описывает подход платформы TensorTalks к конфиденциальности. Мы соблюдаем применимые требования законодательства Российской Федерации в части защиты информации.
              </p>
            </section>

            <section>
              <h2 className="font-semibold text-zinc-900 mb-2">2. Cookies и трекинг</h2>
              <p>
                Мы не используем cookies, не применяем пиксели отслеживания и иные аналогичные технологии для идентификации пользователей.
              </p>
            </section>

            <section>
              <h2 className="font-semibold text-zinc-900 mb-2">3. Персональные данные</h2>
              <p>
                Платформа не запрашивает, не собирает, не обрабатывает и не хранит персональные данные пользователей в смысле Федерального закона РФ № 152‑ФЗ «О персональных данных».
                Регистрационные данные ограничены обезличенным логином и паролем.
              </p>
            </section>

            <section>
              <h2 className="font-semibold text-zinc-900 mb-2">4. Логин пользователя</h2>
              <p>
                Логин должен быть обезличенным и не должен содержать персональные данные, включая, но не ограничиваясь: ФИО, адрес электронной почты, номер телефона, ссылки на профили в соцсетях и т. п.
              </p>
            </section>

            <section>
              <h2 className="font-semibold text-zinc-900 mb-2">5. Безопасность</h2>
              <p>
                Мы рекомендуем использовать сложные пароли и не передавать учетные данные третьим лицам. В случае вопросов по безопасности свяжитесь с нами.
              </p>
            </section>
          </div>

          <div className="mt-6 p-4 rounded-xl bg-gradient-to-r from-orange-50 to-rose-50 border border-orange-100 text-sm text-zinc-700">
            По всем вопросам вы можете связаться с нами через раздел «Связаться с нами» на главной странице.
          </div>
        </div>
      </main>
    </div>
  )
}


