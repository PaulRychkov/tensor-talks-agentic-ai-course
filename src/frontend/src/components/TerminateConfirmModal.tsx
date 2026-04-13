interface TerminateConfirmModalProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
}

export default function TerminateConfirmModal({ isOpen, onClose, onConfirm }: TerminateConfirmModalProps) {
  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div 
        className="bg-white rounded-xl p-6 max-w-md w-full mx-4 shadow-xl" 
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 mb-4">
          <div className="w-10 h-10 rounded-full bg-yellow-100 flex items-center justify-center">
            <span className="text-2xl">⚠️</span>
          </div>
          <h2 className="text-xl font-semibold text-zinc-900">Досрочное завершение интервью</h2>
        </div>
        
        <p className="text-zinc-600 mb-6">
          Вы уверены, что хотите досрочно завершить интервью? Результаты будут сохранены на основе текущего прогресса.
        </p>
        
        <div className="flex gap-3 justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-lg border border-orange-200 hover:bg-orange-50 text-zinc-700"
          >
            Отмена
          </button>
          <button
            onClick={onConfirm}
            className="px-4 py-2 rounded-lg bg-red-600 text-white hover:bg-red-700"
          >
            Завершить интервью
          </button>
        </div>
      </div>
    </div>
  )
}

