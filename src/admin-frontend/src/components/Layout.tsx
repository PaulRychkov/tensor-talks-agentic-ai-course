import { NavLink, useNavigate } from 'react-router-dom'
import { clearToken } from '../services/api'

const NAV = [
  { to: '/', label: 'Дашборд', icon: '📊', end: true },
  { to: '/metrics', label: 'Метрики', icon: '📈' },
  { to: '/questions', label: 'Вопросы', icon: '❓' },
  { to: '/knowledge', label: 'База знаний', icon: '📚' },
  { to: '/upload', label: 'Загрузка', icon: '⬆️' },
  { to: '/drafts', label: 'Черновики', icon: '📝' },
  { to: '/login-words', label: 'Словари', icon: '🔤' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()

  function handleLogout() {
    clearToken()
    navigate('/login')
  }

  return (
    <div className="flex h-full bg-zinc-50">
      {/* Sidebar */}
      <aside className="w-56 flex-shrink-0 bg-white border-r border-zinc-200 flex flex-col">
        <div className="px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-orange-500 to-rose-500" />
            <span className="font-bold text-zinc-800 text-sm">TT Admin</span>
          </div>
        </div>
        <nav className="flex-1 py-3 px-2 space-y-0.5">
          {NAV.map(({ to, label, icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors ${
                  isActive
                    ? 'bg-orange-50 text-orange-700 font-medium'
                    : 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900'
                }`
              }
            >
              <span>{icon}</span>
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="px-3 py-3 border-t border-zinc-100">
          <button
            onClick={handleLogout}
            className="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-sm text-zinc-500 hover:bg-zinc-50 hover:text-zinc-700 transition-colors"
          >
            <span>🚪</span> Выйти
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto">
        {children}
      </main>
    </div>
  )
}
