import { useEffect } from 'react'

interface MVPNotificationProps {
  isOpen: boolean
  onClose: () => void
}

export default function MVPNotification({ isOpen, onClose }: MVPNotificationProps) {
  useEffect(() => {
    if (isOpen) {
      // Автозакрытие через 4 секунды
      const timer = setTimeout(() => {
        onClose()
      }, 4000)
      
      return () => clearTimeout(timer)
    }
  }, [isOpen, onClose])

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Overlay */}
      <div 
        className="absolute inset-0 bg-black/40 backdrop-blur-sm animate-fadeIn"
        onClick={onClose}
      />
      
      {/* Modal */}
      <div className="relative bg-white rounded-3xl border-2 border-orange-200 shadow-2xl p-8 max-w-md w-full animate-slideUp">
        {/* Кнопка закрытия */}
        <button
          onClick={onClose}
          className="absolute top-4 right-4 w-8 h-8 rounded-full bg-orange-100 hover:bg-orange-200 flex items-center justify-center transition-all duration-200 group"
          aria-label="Закрыть"
        >
          <span className="text-orange-600 group-hover:text-orange-700 text-xl leading-none">×</span>
        </button>
        
        {/* Иконка */}
        <div className="flex justify-center mb-6">
          <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-orange-400 to-rose-500 flex items-center justify-center shadow-lg animate-bounce">
            <span className="text-4xl">🚀</span>
          </div>
        </div>
        
        {/* Контент */}
        <div className="text-center">
          <h3 className="text-2xl font-bold text-zinc-900 mb-3">
            Скоро здесь всё заработает!
          </h3>
          <p className="text-zinc-600 leading-relaxed mb-6">
            Это лендинг проекта <span className="font-semibold text-orange-600">TensorTalks</span>. 
            Полноценный MVP находится в активной разработке и уже совсем скоро будет готов! 🎉
          </p>
          
          {/* Прогресс бар */}
          <div className="mb-6">
            <div className="flex justify-between text-xs text-zinc-500 mb-2">
              <span>Готовность MVP</span>
              <span className="font-semibold text-orange-600">85%</span>
            </div>
            <div className="h-2 bg-orange-100 rounded-full overflow-hidden">
              <div className="h-full w-[85%] bg-gradient-to-r from-orange-500 to-rose-500 rounded-full animate-pulse" />
            </div>
          </div>
          
          {/* Ключевые пункты */}
          <div className="grid grid-cols-2 gap-3 mb-6 text-sm">
            <div className="flex items-center gap-2 p-3 rounded-xl bg-gradient-to-br from-blue-50 to-blue-100 border border-blue-200">
              <span className="text-blue-600 text-lg">🤖</span>
              <span className="text-blue-900 font-medium">AI-интервьюер</span>
            </div>
            <div className="flex items-center gap-2 p-3 rounded-xl bg-gradient-to-br from-green-50 to-green-100 border border-green-200">
              <span className="text-green-600 text-lg">📊</span>
              <span className="text-green-900 font-medium">Аналитика</span>
            </div>
            <div className="flex items-center gap-2 p-3 rounded-xl bg-gradient-to-br from-purple-50 to-purple-100 border border-purple-200">
              <span className="text-purple-600 text-lg">💡</span>
              <span className="text-purple-900 font-medium">Рекомендации</span>
            </div>
            <div className="flex items-center gap-2 p-3 rounded-xl bg-gradient-to-br from-orange-50 to-orange-100 border border-orange-200">
              <span className="text-orange-600 text-lg">🎯</span>
              <span className="text-orange-900 font-medium">База вопросов</span>
            </div>
          </div>
          
          {/* Призыв к действию */}
          <div className="p-4 rounded-xl bg-gradient-to-r from-orange-50 to-rose-50 border border-orange-200">
            <p className="text-sm text-zinc-700">
              Следите за обновлениями — мы запустимся в <span className="font-bold text-orange-600">2025 году</span>!
            </p>
          </div>
        </div>
        
        {/* Автозакрытие индикатор */}
        <div className="mt-6 flex items-center justify-center gap-2 text-xs text-zinc-400">
          <span className="w-1.5 h-1.5 bg-orange-400 rounded-full animate-pulse"></span>
          <span>Автоматически закроется через 4 секунды</span>
        </div>
      </div>
      
      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; }
          to { opacity: 1; }
        }
        
        @keyframes slideUp {
          from {
            opacity: 0;
            transform: translateY(20px) scale(0.95);
          }
          to {
            opacity: 1;
            transform: translateY(0) scale(1);
          }
        }
        
        .animate-fadeIn {
          animation: fadeIn 0.3s ease-out;
        }
        
        .animate-slideUp {
          animation: slideUp 0.4s cubic-bezier(0.16, 1, 0.3, 1);
        }
      `}</style>
    </div>
  )
}

