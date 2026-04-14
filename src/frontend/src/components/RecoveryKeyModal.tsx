/**
 * Modal shown once after registration with the recovery key (§10.10).
 * Key is shown ONCE and must be saved by the user.
 */

type Props = {
  recoveryKey: string
  onClose: () => void
}

export default function RecoveryKeyModal({ recoveryKey, onClose }: Props) {
  const handleCopy = () => {
    navigator.clipboard.writeText(recoveryKey).catch(() => {})
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm px-4">
      <div className="bg-white rounded-2xl shadow-xl p-6 max-w-md w-full">
        <div className="text-center mb-4">
          <div className="text-4xl mb-2">✓</div>
          <h2 className="text-xl font-bold text-gray-800">Регистрация завершена!</h2>
        </div>

        <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 mb-4">
          <p className="text-sm font-medium text-amber-800 mb-1">⚠ Сохраните ключ восстановления</p>
          <p className="text-xs text-amber-700 leading-relaxed">
            Он понадобится, если вы забудете пароль. Ключ показывается{' '}
            <strong>ОДИН РАЗ</strong> и не может быть восстановлен.
          </p>
        </div>

        <div className="flex items-center gap-2 bg-gray-100 rounded-lg px-3 py-2 mb-4">
          <code className="text-sm text-gray-800 flex-1 break-all font-mono">{recoveryKey}</code>
          <button
            onClick={handleCopy}
            title="Скопировать"
            className="text-orange-500 hover:text-orange-700 shrink-0 text-lg"
          >
            📋
          </button>
        </div>

        <div className="flex gap-2">
          <button
            onClick={handleCopy}
            className="flex-1 border border-orange-400 text-orange-500 hover:bg-orange-50 rounded-lg px-4 py-2 text-sm font-medium transition-colors"
          >
            📋 Скопировать
          </button>
          <button
            onClick={onClose}
            className="flex-1 bg-orange-500 hover:bg-orange-600 text-white rounded-lg px-4 py-2 text-sm font-medium transition-colors"
          >
            Я сохранил →
          </button>
        </div>
      </div>
    </div>
  )
}
