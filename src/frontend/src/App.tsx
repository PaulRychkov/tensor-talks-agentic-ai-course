import { Link } from 'react-router-dom'
import { useState, useEffect } from 'react'
import MVPNotification from './components/MVPNotification'

export default function App() {
  const [showMVPPopup, setShowMVPPopup] = useState(false)
  const [showScrollIndicator, setShowScrollIndicator] = useState(true)
  
  const handleFeatureClick = (e: React.MouseEvent) => {
    e.preventDefault()
    setShowMVPPopup(true)
  }
  
  const scrollToSection = (sectionId: string) => {
    const element = document.getElementById(sectionId)
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }

  useEffect(() => {
    const handleScroll = () => {
      if (window.scrollY > 100) {
        setShowScrollIndicator(false)
      }
    }

    window.addEventListener('scroll', handleScroll)
    return () => window.removeEventListener('scroll', handleScroll)
  }, [])
  
  return (
    <div className="bg-gradient-to-b from-orange-50 to-white text-zinc-900">
      <MVPNotification isOpen={showMVPPopup} onClose={() => setShowMVPPopup(false)} />
      <header className="border-b border-orange-100 bg-white/70 backdrop-blur fixed top-0 left-0 right-0 z-10">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="size-8 rounded-lg bg-gradient-to-br from-orange-500 to-rose-500" />
            <h1 className="text-xl font-bold">TensorTalks</h1>
          </div>
          <nav className="hidden sm:flex items-center gap-5 text-sm text-zinc-600">
            <button onClick={() => scrollToSection('how-it-works')} className="hover:text-zinc-900">Как это работает</button>
            <button onClick={() => scrollToSection('features')} className="hover:text-zinc-900">Фичи</button>
            <button onClick={() => scrollToSection('pricing')} className="hover:text-zinc-900">Тарифы</button>
            <button onClick={() => scrollToSection('faq')} className="hover:text-zinc-900">FAQ</button>
          </nav>
          <div className="flex items-center gap-2">
            <Link to="/auth" className="px-4 py-2 rounded-lg bg-orange-600 text-white hover:bg-orange-700">Регистрация</Link>
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 pt-24 pb-10">
        {/* Hero Section */}
        <section className="relative overflow-hidden rounded-3xl border border-orange-100 bg-gradient-to-br from-orange-50 via-white to-rose-50 p-8 md:p-16 shadow-2xl">
          <div className="max-w-4xl">
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-orange-100 text-orange-700 text-sm font-medium mb-6">
              <span className="w-2 h-2 bg-orange-500 rounded-full animate-pulse"></span>
              AI-симулятор технических интервью
            </div>
            <h1 className="text-4xl md:text-6xl font-extrabold mb-6 bg-gradient-to-r from-orange-600 via-rose-600 to-orange-800 bg-clip-text text-transparent leading-tight">
              TensorTalks — объективная оценка ML-компетенций
            </h1>
            <p className="text-xl text-zinc-700 mb-8 leading-relaxed">
              Умный тренажёр технических интервью для ML‑специалистов и команд. 
              Моделируем реальные собеседования с AI‑интервьюером, анализируем ответы, 
              выявляем слабые стороны и предлагаем индивидуальные рекомендации.
            </p>
            
            {/* Key Benefits */}
            <div className="grid md:grid-cols-4 gap-3 mb-8">
              <div className="flex items-center gap-2 p-3 rounded-lg bg-white/60 backdrop-blur-sm border border-orange-100">
                <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-green-400 to-green-600 flex items-center justify-center flex-shrink-0">
                  <span className="text-white text-sm">⚡</span>
                </div>
                <div className="min-w-0">
                  <div className="font-semibold text-sm">Значительно быстрее</div>
                  <div className="text-xs text-zinc-600">подготовка к интервью</div>
                </div>
              </div>
              <div className="flex items-center gap-2 p-3 rounded-lg bg-white/60 backdrop-blur-sm border border-orange-100">
                <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center flex-shrink-0">
                  <span className="text-white text-sm">🎯</span>
                </div>
                <div className="min-w-0">
                  <div className="font-semibold text-sm">Объективная оценка</div>
                  <div className="text-xs text-zinc-600">технических навыков</div>
                </div>
              </div>
              <div className="flex items-center gap-2 p-3 rounded-lg bg-white/60 backdrop-blur-sm border border-orange-100">
                <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-purple-400 to-purple-600 flex items-center justify-center flex-shrink-0">
                  <span className="text-white text-sm">📈</span>
                </div>
                <div className="min-w-0">
                  <div className="font-semibold text-sm">Персонализация</div>
                  <div className="text-xs text-zinc-600">обучения и развития</div>
                </div>
              </div>
              <div className="flex items-center gap-2 p-3 rounded-lg bg-white/60 backdrop-blur-sm border border-orange-100">
                <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-red-400 to-red-600 flex items-center justify-center flex-shrink-0">
                  <span className="text-white text-sm">🔍</span>
                </div>
                <div className="min-w-0">
                  <div className="font-semibold text-sm">Выявление пробелов</div>
                  <div className="text-xs text-zinc-600">в знаниях</div>
                </div>
              </div>
            </div>
            
            <div className="flex flex-wrap gap-4">
              <Link to="/auth" className="px-6 py-3 rounded-xl bg-gradient-to-r from-orange-600 to-rose-600 text-white hover:from-orange-700 hover:to-rose-700 font-semibold shadow-lg hover:shadow-xl transition-all duration-300">
                Начать бесплатно
              </Link>
              <button onClick={() => scrollToSection('segments')} className="px-6 py-3 rounded-xl border border-orange-200 hover:bg-orange-50 font-semibold transition-all duration-300">
                Узнать больше
              </button>
            </div>
          </div>
          
          {/* Decorative Elements */}
          <div className="pointer-events-none absolute -right-20 -top-20 size-60 md:size-96 rounded-full bg-gradient-to-tr from-orange-300/30 to-rose-300/30 blur-3xl animate-pulse" />
          <div className="pointer-events-none absolute -left-10 -bottom-10 size-40 md:size-64 rounded-full bg-gradient-to-tr from-blue-300/20 to-purple-300/20 blur-3xl" />
          <div className="pointer-events-none absolute top-1/2 right-1/4 size-20 md:size-32 rounded-full bg-gradient-to-tr from-green-300/20 to-blue-300/20 blur-2xl animate-bounce" />
        </section>


        {/* Problem Statement Section */}
        <section className="mt-16">
          <div className="relative overflow-hidden rounded-2xl bg-gradient-to-br from-orange-100 via-rose-100 to-amber-100 py-6 px-2 md:px-3 shadow-lg z-0">
            <div className="relative z-10 max-w-5xl mx-auto">
              <div className="space-y-3 mb-6">
                <div className="p-4 rounded-xl bg-white/60 backdrop-blur-sm border border-orange-200 hover:bg-white/80 transition-all duration-300">
                  <div className="flex items-start gap-3">
                    <span className="text-2xl flex-shrink-0">💼</span>
                    <p className="text-lg md:text-xl font-medium text-zinc-800">
                      Вы устали получать множество отказов без обратной связи и не знаете как готовиться к собеседованиям, закрыть пробелы в знаниях?
                    </p>
                  </div>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 backdrop-blur-sm border border-orange-200 hover:bg-white/80 transition-all duration-300">
                  <div className="flex items-start gap-3">
                    <span className="text-2xl flex-shrink-0">🎯</span>
                    <p className="text-lg md:text-xl font-medium text-zinc-800">
                      Вы HR который устал сталкиваться с накруткой опыта?
                    </p>
                  </div>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 backdrop-blur-sm border border-orange-200 hover:bg-white/80 transition-all duration-300">
                  <div className="flex items-start gap-3">
                    <span className="text-2xl flex-shrink-0">📊</span>
                    <p className="text-lg md:text-xl font-medium text-zinc-800">
                      Вы руководитель и не знаете как оценить компетенции и развитие вашей команды в цифрах, понять точки роста?
                    </p>
                  </div>
                </div>
              </div>
              
              <div className="text-center mb-6">
                <h3 className="text-2xl md:text-3xl font-bold text-zinc-900">
                  Тогда TensorTalks создан для вас
                </h3>
              </div>
              
              <div className="flex flex-col sm:flex-row gap-3 justify-center items-center">
                <button 
                  onClick={handleFeatureClick}
                  className="w-full sm:w-64 px-6 py-3 rounded-xl bg-gradient-to-r from-orange-500 to-rose-500 text-white font-semibold hover:from-orange-600 hover:to-rose-600 transition-all duration-300 shadow-lg hover:shadow-xl"
                >
                  Связаться с нами
                </button>
                <Link 
                  to="/auth"
                  className="w-full sm:w-64 px-6 py-3 rounded-xl bg-gradient-to-r from-orange-500 to-rose-500 text-white font-semibold hover:from-orange-600 hover:to-rose-600 transition-all duration-300 shadow-lg hover:shadow-xl text-center"
                >
                  Попробовать бесплатно
                </Link>
              </div>
            </div>
            
            {/* Decorative Elements */}
            <div className="pointer-events-none absolute -right-20 -top-20 size-40 md:size-60 rounded-full bg-gradient-to-tr from-orange-200/30 to-rose-200/30 blur-3xl" />
            <div className="pointer-events-none absolute -left-10 -bottom-10 size-32 md:size-48 rounded-full bg-gradient-to-tr from-amber-200/20 to-orange-200/20 blur-3xl" />
          </div>
        </section>

        {/* Client Segments Section */}
        <section id="segments" className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Для кого создан TensorTalks</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Платформа решает задачи разных участников ML-экосистемы — от начинающих специалистов до крупных команд
            </p>
          </div>
          
          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
            {/* Individual ML Specialists */}
            <div className="group relative p-6 rounded-2xl bg-gradient-to-br from-blue-50 to-blue-100 border border-blue-200 hover:shadow-xl transition-all duration-300">
              <div className="absolute top-4 right-4 w-12 h-12 rounded-xl bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center group-hover:scale-110 transition-transform duration-300">
                <span className="text-white text-xl">👨‍💻</span>
              </div>
              <h3 className="text-lg font-bold mb-2 text-blue-900">ML-специалисты</h3>
              <p className="text-sm text-blue-700 mb-4">Middle-to-Senior уровень, готовящиеся к техническим интервью</p>
              <div className="space-y-2 text-xs text-blue-600">
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full"></span>
                  Сложно оценить свой уровень
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full"></span>
                  Недостаток практики
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-blue-500 rounded-full"></span>
                  Нет структурированного фидбэка
                </div>
              </div>
            </div>

            {/* Juniors & Transitioning */}
            <div className="group relative p-6 rounded-2xl bg-gradient-to-br from-green-50 to-green-100 border border-green-200 hover:shadow-xl transition-all duration-300">
              <div className="absolute top-4 right-4 w-12 h-12 rounded-xl bg-gradient-to-br from-green-400 to-green-600 flex items-center justify-center group-hover:scale-110 transition-transform duration-300">
                <span className="text-white text-xl">🌱</span>
              </div>
              <h3 className="text-lg font-bold mb-2 text-green-900">Новички в ML</h3>
              <p className="text-sm text-green-700 mb-4">Разработчики и аналитики, переходящие в ML</p>
              <div className="space-y-2 text-xs text-green-600">
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>
                  Не знают, с чего начать
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>
                  Хотят понять требования
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>
                  Нужен план подготовки
                </div>
              </div>
            </div>

            {/* Hiring Companies */}
            <div className="group relative p-6 rounded-2xl bg-gradient-to-br from-purple-50 to-purple-100 border border-purple-200 hover:shadow-xl transition-all duration-300">
              <div className="absolute top-4 right-4 w-12 h-12 rounded-xl bg-gradient-to-br from-purple-400 to-purple-600 flex items-center justify-center group-hover:scale-110 transition-transform duration-300">
                <span className="text-white text-xl">🏢</span>
              </div>
              <h3 className="text-lg font-bold mb-2 text-purple-900">Компании (Hiring)</h3>
              <p className="text-sm text-purple-700 mb-4">ML-команды и HR Tech-рекрутеры</p>
              <div className="space-y-2 text-xs text-purple-600">
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-purple-500 rounded-full"></span>
                  Разный уровень интервьюеров
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-purple-500 rounded-full"></span>
                  Субъективная оценка
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-purple-500 rounded-full"></span>
                  Много времени на подготовку
                </div>
              </div>
            </div>

            {/* L&D Companies */}
            <div className="group relative p-6 rounded-2xl bg-gradient-to-br from-orange-50 to-orange-100 border border-orange-200 hover:shadow-xl transition-all duration-300">
              <div className="absolute top-4 right-4 w-12 h-12 rounded-xl bg-gradient-to-br from-orange-400 to-orange-600 flex items-center justify-center group-hover:scale-110 transition-transform duration-300">
                <span className="text-white text-xl">📚</span>
              </div>
              <h3 className="text-lg font-bold mb-2 text-orange-900">Компании (L&D)</h3>
              <p className="text-sm text-orange-700 mb-4">Team Leads и HR/L&D, развивающие команды</p>
              <div className="space-y-2 text-xs text-orange-600">
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-orange-500 rounded-full"></span>
                  Нет системной диагностики
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-orange-500 rounded-full"></span>
                  Хотят отслеживать прогресс
                </div>
                <div className="flex items-center gap-2">
                  <span className="w-1.5 h-1.5 bg-orange-500 rounded-full"></span>
                  Закрыть пробелы целенаправленно
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* Value Proposition Section */}
        <section className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Основная ценность продукта</h2>
            <div className="max-w-4xl mx-auto p-8 rounded-3xl bg-gradient-to-r from-orange-100 to-rose-100 border border-orange-200">
              <p className="text-xl text-zinc-700 leading-relaxed">
                <span className="font-bold text-orange-700">TensorTalks помогает ML‑специалистам и компаниям объективно оценивать и развивать технические компетенции в области машинного обучения</span> — через реалистичные AI‑симуляции интервью, персонализированный разбор и аналитику сильных и слабых сторон.
              </p>
            </div>
          </div>
        </section>

        <section id="how-it-works" className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Как это работает</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Простой процесс для максимальной эффективности подготовки к ML-интервью
            </p>
          </div>
          
          <div className="grid md:grid-cols-3 gap-8 relative">
            {/* Connection lines for desktop */}
            <div className="hidden md:block absolute top-16 left-1/3 right-1/3 h-0.5 bg-gradient-to-r from-orange-200 to-rose-200"></div>
            <div className="hidden md:block absolute top-16 left-2/3 right-0 h-0.5 bg-gradient-to-r from-rose-200 to-orange-200"></div>
            
            {[{
              title: '1. Выберите уровень',
              text: 'Определите свой уровень: Junior, Middle или Senior ML-специалист.',
              icon: '🎯',
              color: 'blue'
            }, {
              title: '2. Пройдите интервью',
              text: 'Отвечайте на вопросы по алгоритмам, машинному обучению и практическим кейсам.',
              icon: '💬',
              color: 'green'
            }, {
              title: '3. Получите обратную связь',
              text: 'Анализируйте результаты, изучайте разборы ошибок и улучшайте навыки.',
              icon: '📊',
              color: 'purple'
            }].map((f, index) => (
              <div key={f.title} className="relative">
                <div className={`rounded-2xl border p-8 bg-white shadow-lg hover:shadow-xl transition-all duration-300 group ${f.color === 'blue' ? 'border-blue-200 hover:border-blue-300' : f.color === 'green' ? 'border-green-200 hover:border-green-300' : 'border-purple-200 hover:border-purple-300'}`}>
                  <div className={`size-16 rounded-2xl flex items-center justify-center mb-4 text-2xl ${f.color === 'blue' ? 'bg-gradient-to-br from-blue-100 to-blue-200' : f.color === 'green' ? 'bg-gradient-to-br from-green-100 to-green-200' : 'bg-gradient-to-br from-purple-100 to-purple-200'} group-hover:scale-110 transition-transform duration-300`}>
                    {f.icon}
                  </div>
                  <h3 className="text-xl font-bold mb-3">{f.title}</h3>
                  <p className="text-zinc-600 leading-relaxed">{f.text}</p>
                </div>
                
                {/* Step number */}
                <div className={`absolute -top-4 -right-4 size-8 rounded-full flex items-center justify-center text-white font-bold text-sm ${f.color === 'blue' ? 'bg-blue-500' : f.color === 'green' ? 'bg-green-500' : 'bg-purple-500'}`}>
                  {index + 1}
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* Benefits Section */}
        <section className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Преимущества и измеримые выгоды</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Конкретные результаты, которые получают наши пользователи
            </p>
          </div>
          
          <div className="grid md:grid-cols-2 gap-8">
            {/* For Individuals */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-blue-50 to-blue-100 border border-blue-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center">
                  <span className="text-white text-xl">👨‍💻</span>
                </div>
                <h3 className="text-2xl font-bold text-blue-900">Для ML-специалистов</h3>
              </div>
              
              <div className="space-y-6">
                <div className="p-4 rounded-xl bg-white/60 border border-blue-100">
                  <h4 className="font-semibold text-blue-800 mb-2">Преимущества продукта</h4>
                  <ul className="space-y-2 text-sm text-blue-700">
                    <li className="flex items-start gap-2">
                      <span className="text-blue-500 mt-1">•</span>
                      Реальные ML-вопросы уровня FAANG и стартапов
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-blue-500 mt-1">•</span>
                      Разбор ответов и ошибок от AI-интервьюера
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-blue-500 mt-1">•</span>
                      Индивидуальные рекомендации по темам
                    </li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-blue-100">
                  <h4 className="font-semibold text-blue-800 mb-2">Измеримые выгоды</h4>
                  <ul className="space-y-2 text-sm text-blue-700">
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↓</span>
                      <span>Значительно меньше времени на подготовку</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↑</span>
                      <span>Улучшение результатов на реальных собеседованиях</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↑</span>
                      <span>Повышение уверенности и качества ответов</span>
                    </li>
                  </ul>
                </div>
              </div>
            </div>

            {/* For Companies */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-purple-50 to-purple-100 border border-purple-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-purple-400 to-purple-600 flex items-center justify-center">
                  <span className="text-white text-xl">🏢</span>
                </div>
                <h3 className="text-2xl font-bold text-purple-900">Для компаний</h3>
              </div>
              
              <div className="space-y-6">
                <div className="p-4 rounded-xl bg-white/60 border border-purple-100">
                  <h4 className="font-semibold text-purple-800 mb-2">Преимущества продукта</h4>
                  <ul className="space-y-2 text-sm text-purple-700">
                    <li className="flex items-start gap-2">
                      <span className="text-purple-500 mt-1">•</span>
                      Унифицированная база заданий и критериев
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-purple-500 mt-1">•</span>
                      Автоматизированные AI-интервью
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-purple-500 mt-1">•</span>
                      Аналитика по компетенциям кандидатов
                    </li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-purple-100">
                  <h4 className="font-semibold text-purple-800 mb-2">Измеримые выгоды</h4>
                  <ul className="space-y-2 text-sm text-purple-700">
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↓</span>
                      <span>Значительное сокращение времени интервью</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↑</span>
                      <span>Повышение точности отбора</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 font-bold">↓</span>
                      <span>Минимизация субъективности оценок</span>
                    </li>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section id="features" className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Ключевые возможности</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Все необходимые инструменты для эффективной подготовки и оценки ML-компетенций
            </p>
          </div>
          
          <div className="grid md:grid-cols-3 gap-8">
            {[{
              title: 'База вопросов',
              text: 'Алгоритмы, ML‑задачи, практические кейсы по уровням сложности от Junior до Senior.',
              icon: '📚',
              color: 'blue',
              features: ['Обширная база вопросов', 'FAANG уровень', 'Актуальные технологии']
            }, {
              title: 'AI-разбор ошибок',
              text: 'Детальная обратная связь с объяснением ошибок и рекомендациями по улучшению.',
              icon: '🔍',
              color: 'green',
              features: ['Персонализированный анализ', 'Конкретные рекомендации', 'Материалы для изучения']
            }, {
              title: 'Стандартизация оценки',
              text: 'Единый формат оценки для HR и техлидов с объективными критериями.',
              icon: '⚖️',
              color: 'purple',
              features: ['Объективные метрики', 'Сравнимые результаты', 'Отчеты для команд']
            }].map((f) => (
              <div key={f.title} className="group relative">
                <div className={`rounded-2xl border p-8 bg-white shadow-lg hover:shadow-xl transition-all duration-300 h-full ${f.color === 'blue' ? 'border-blue-200 hover:border-blue-300' : f.color === 'green' ? 'border-green-200 hover:border-green-300' : 'border-purple-200 hover:border-purple-300'}`}>
                  <div className={`size-16 rounded-2xl flex items-center justify-center mb-6 text-2xl ${f.color === 'blue' ? 'bg-gradient-to-br from-blue-100 to-blue-200' : f.color === 'green' ? 'bg-gradient-to-br from-green-100 to-green-200' : 'bg-gradient-to-br from-purple-100 to-purple-200'} group-hover:scale-110 transition-transform duration-300`}>
                    {f.icon}
                  </div>
                  <h3 className="text-xl font-bold mb-3">{f.title}</h3>
                  <p className="text-zinc-600 mb-6 leading-relaxed">{f.text}</p>
                  
                  <div className="space-y-2">
                    {f.features.map((feature) => (
                      <div key={feature} className="flex items-center gap-2 text-sm">
                        <div className={`w-1.5 h-1.5 rounded-full ${f.color === 'blue' ? 'bg-blue-500' : f.color === 'green' ? 'bg-green-500' : 'bg-purple-500'}`}></div>
                        <span className="text-zinc-600">{feature}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>

        <section id="pricing" className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Тарифы</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Выберите подходящий план для ваших потребностей в ML-подготовке
            </p>
          </div>
          
          <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto">
            {[
              { 
                name: 'Free', 
                price: '0 ₽', 
                period: 'навсегда',
                description: 'Для начинающих',
                features: ['10 вопросов в день', 'Базовые отчёты', '1 неделя Pro trial', 'Доступ к сообществу'],
                color: 'gray',
                popular: false
              },
              { 
                name: 'Pro', 
                price: '990 ₽', 
                period: 'в месяц',
                description: 'Для ML-специалистов',
                features: ['Безлимитные интервью', 'Подробный AI-разбор', 'История прогресса', 'Персональные рекомендации', 'Приоритетная поддержка'],
                color: 'orange',
                popular: true
              },
              { 
                name: 'Enterprise', 
                price: '2990 ₽', 
                period: 'в месяц',
                description: 'Для команд и HR',
                features: ['Командные отчёты', 'Роли HR/TechLead', 'Управление кандидатами', 'API интеграции', 'Персональный менеджер', 'Кастомные оценки'],
                color: 'purple',
                popular: false
              },
            ].map((p) => (
              <div key={p.name} className={`relative rounded-3xl border p-8 bg-white shadow-lg hover:shadow-xl transition-all duration-300 ${p.popular ? 'ring-2 ring-orange-500 scale-105' : ''} ${p.color === 'gray' ? 'border-gray-200' : p.color === 'orange' ? 'border-orange-200' : 'border-purple-200'}`}>
                {p.popular && (
                  <div className="absolute -top-4 left-1/2 transform -translate-x-1/2">
                    <span className="px-4 py-1 rounded-full bg-gradient-to-r from-orange-500 to-rose-500 text-white text-sm font-medium">
                      Популярный
                    </span>
                  </div>
                )}
                
                <div className="text-center mb-6">
                  <h3 className="text-2xl font-bold mb-2">{p.name}</h3>
                  <p className="text-zinc-600 mb-4">{p.description}</p>
                  <div className="flex items-baseline justify-center gap-1">
                    <span className="text-4xl font-bold text-zinc-900">{p.price}</span>
                    <span className="text-zinc-600">/{p.period}</span>
                  </div>
                </div>
                
                <ul className="space-y-4 mb-8">
                  {p.features.map((f) => (
                    <li key={f} className="flex items-start gap-3">
                      <div className={`w-5 h-5 rounded-full flex items-center justify-center mt-0.5 ${p.color === 'gray' ? 'bg-gray-100' : p.color === 'orange' ? 'bg-orange-100' : 'bg-purple-100'}`}>
                        <span className="text-xs">✓</span>
                      </div>
                      <span className="text-zinc-700">{f}</span>
                    </li>
                  ))}
                </ul>
                
                <Link 
                  to="/auth" 
                  className={`w-full inline-block text-center px-6 py-3 rounded-xl font-semibold transition-all duration-300 ${
                    p.color === 'gray' 
                      ? 'bg-gray-100 text-gray-900 hover:bg-gray-200' 
                      : p.color === 'orange' 
                      ? 'bg-gradient-to-r from-orange-600 to-rose-600 text-white hover:from-orange-700 hover:to-rose-700 shadow-lg hover:shadow-xl' 
                      : 'bg-gradient-to-r from-purple-600 to-purple-700 text-white hover:from-purple-700 hover:to-purple-800 shadow-lg hover:shadow-xl'
                  }`}
                >
                  Выбрать план
                </Link>
              </div>
            ))}
          </div>
          
          <div className="text-center mt-12">
            <p className="text-zinc-600 mb-4">Нужен индивидуальный план для вашей компании?</p>
            <button onClick={handleFeatureClick} className="inline-flex items-center gap-2 px-6 py-3 rounded-xl border border-orange-200 hover:bg-orange-50 font-semibold transition-all duration-300">
              <span>📧</span>
              Связаться с нами
            </button>
          </div>
        </section>

        {/* User Insights Section */}
        <section className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Инсайты от целевых групп</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Понимание потребностей и ожиданий от TensorTalks на основе проведенных интервью
            </p>
          </div>
          
          <div className="grid md:grid-cols-2 gap-8">
            {/* Individual ML Specialists Insights */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-blue-50 to-blue-100 border border-blue-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center">
                  <span className="text-white text-xl">👨‍💻</span>
                </div>
                <h3 className="text-2xl font-bold text-blue-900">ML-специалисты</h3>
              </div>
              
              <div className="space-y-4">
                <div className="p-4 rounded-xl bg-white/60 border border-blue-100">
                  <h4 className="font-semibold text-blue-800 mb-2">Основные потребности</h4>
                  <ul className="space-y-2 text-sm text-blue-700">
                    <li>• Объективная оценка своего уровня</li>
                    <li>• Практика в условиях реального интервью</li>
                    <li>• Структурированная обратная связь</li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-blue-100">
                  <h4 className="font-semibold text-blue-800 mb-2">Ожидания от MVP</h4>
                  <ul className="space-y-2 text-sm text-blue-700">
                    <li>• Вопросы уровня FAANG и топ-стартапов</li>
                    <li>• Детальный разбор ошибок</li>
                    <li>• Рекомендации по темам для изучения</li>
                  </ul>
                </div>
              </div>
            </div>

            {/* Juniors & Transitioning Insights */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-green-50 to-green-100 border border-green-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-green-400 to-green-600 flex items-center justify-center">
                  <span className="text-white text-xl">🌱</span>
                </div>
                <h3 className="text-2xl font-bold text-green-900">Новички в ML</h3>
              </div>
              
              <div className="space-y-4">
                <div className="p-4 rounded-xl bg-white/60 border border-green-100">
                  <h4 className="font-semibold text-green-800 mb-2">Основные потребности</h4>
                  <ul className="space-y-2 text-sm text-green-700">
                    <li>• Понять требования к ML-интервью</li>
                    <li>• Получить структурированный план подготовки</li>
                    <li>• Наставничество и гайды</li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-green-100">
                  <h4 className="font-semibold text-green-800 mb-2">Ожидания от MVP</h4>
                  <ul className="space-y-2 text-sm text-green-700">
                    <li>• Roadmap по уровням сложности</li>
                    <li>• Объяснения базовых концепций</li>
                    <li>• Пошаговые инструкции</li>
                  </ul>
                </div>
              </div>
            </div>

            {/* Hiring Companies Insights */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-purple-50 to-purple-100 border border-purple-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-purple-400 to-purple-600 flex items-center justify-center">
                  <span className="text-white text-xl">🏢</span>
                </div>
                <h3 className="text-2xl font-bold text-purple-900">Компании (Hiring)</h3>
              </div>
              
              <div className="space-y-4">
                <div className="p-4 rounded-xl bg-white/60 border border-purple-100">
                  <h4 className="font-semibold text-purple-800 mb-2">Основные потребности</h4>
                  <ul className="space-y-2 text-sm text-purple-700">
                    <li>• Стандартизация процесса оценки</li>
                    <li>• Экономия времени интервьюеров</li>
                    <li>• Объективные критерии отбора</li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-purple-100">
                  <h4 className="font-semibold text-purple-800 mb-2">Ожидания от MVP</h4>
                  <ul className="space-y-2 text-sm text-purple-700">
                    <li>• Единая база вопросов и критериев</li>
                    <li>• Автоматизированные отчеты</li>
                    <li>• Сравнимые результаты кандидатов</li>
                  </ul>
                </div>
              </div>
            </div>

            {/* L&D Companies Insights */}
            <div className="p-8 rounded-3xl bg-gradient-to-br from-orange-50 to-orange-100 border border-orange-200">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-orange-400 to-orange-600 flex items-center justify-center">
                  <span className="text-white text-xl">📚</span>
                </div>
                <h3 className="text-2xl font-bold text-orange-900">Компании (L&D)</h3>
              </div>
              
              <div className="space-y-4">
                <div className="p-4 rounded-xl bg-white/60 border border-orange-100">
                  <h4 className="font-semibold text-orange-800 mb-2">Основные потребности</h4>
                  <ul className="space-y-2 text-sm text-orange-700">
                    <li>• Системная диагностика навыков</li>
                    <li>• Отслеживание прогресса команды</li>
                    <li>• Целенаправленное развитие</li>
                  </ul>
                </div>
                
                <div className="p-4 rounded-xl bg-white/60 border border-orange-100">
                  <h4 className="font-semibold text-orange-800 mb-2">Ожидания от MVP</h4>
                  <ul className="space-y-2 text-sm text-orange-700">
                    <li>• Регулярные оценки сотрудников</li>
                    <li>• Персональные планы развития</li>
                    <li>• Аналитика по командам</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section id="faq" className="mt-20 pt-20">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4">Часто задаваемые вопросы</h2>
            <p className="text-xl text-zinc-600 max-w-3xl mx-auto">
              Ответы на ключевые вопросы о концепции и возможностях TensorTalks
            </p>
          </div>
          
          <div className="grid md:grid-cols-2 gap-6 max-w-4xl mx-auto">
            {[
              {
                question: 'Чем TensorTalks отличается от других платформ подготовки?',
                answer: 'TensorTalks фокусируется исключительно на технических ML-интервью с AI-интервьюером, который моделирует реальные условия собеседования и предоставляет детальную обратную связь.'
              },
              {
                question: 'Какой уровень ML-знаний требуется для использования?',
                answer: 'Платформа адаптирована для всех уровней: от новичков, переходящих в ML, до Senior-специалистов. Вопросы и задачи подбираются в зависимости от вашего уровня.'
              },
              {
                question: 'Как работает AI-интервьюер и насколько он реалистичен?',
                answer: 'AI-интервьюер использует современные языковые модели для создания естественного диалога, анализа ответов и выявления пробелов в знаниях, максимально приближенно к реальному интервью.'
              },
              {
                question: 'Какие типы вопросов и задач включены?',
                answer: 'База включает алгоритмы, машинное обучение, системный дизайн ML-систем, практические кейсы и вопросы по современным технологиям, основанные на реальных интервью в FAANG и топ-стартапах.'
              },
              {
                question: 'Как TensorTalks поможет компаниям в найме?',
                answer: 'Для компаний платформа обеспечивает стандартизированную оценку кандидатов, объективные критерии отбора и экономию времени интервьюеров через автоматизированные AI-интервью.'
              },
              {
                question: 'Когда будет доступна MVP версия?',
                answer: 'Мы работаем над MVP и планируем запустить бета-версию в 2025 году. Следите за обновлениями и присоединяйтесь к раннему доступу для тестирования функций.'
              },
              {
                question: 'Будет ли доступна интеграция с HR-системами?',
                answer: 'В планах интеграция с популярными ATS и HR-системами для Enterprise-клиентов, что позволит автоматизировать процесс оценки кандидатов.'
              },
              {
                question: 'Как TensorTalks поможет в развитии команды?',
                answer: 'Для L&D платформа предоставляет системную диагностику навыков сотрудников, отслеживание прогресса и создание персональных планов развития на основе выявленных пробелов.'
              }
            ].map((faq) => (
              <div key={faq.question} className="p-6 rounded-2xl border border-orange-100 bg-white hover:shadow-lg transition-all duration-300">
                <h3 className="font-semibold text-lg mb-3 text-zinc-900">{faq.question}</h3>
                <p className="text-zinc-600 leading-relaxed">{faq.answer}</p>
              </div>
            ))}
          </div>
        </section>

        {/* Final CTA Section */}
        <section className="mt-20 pt-20">
          <div className="relative overflow-hidden rounded-3xl bg-gradient-to-br from-orange-500 via-rose-500 to-orange-600 p-12 md:p-16 text-center text-white">
            <div className="relative z-10 max-w-3xl mx-auto">
              <h2 className="text-3xl md:text-4xl font-bold mb-6">
                Готовы улучшить свои ML-навыки?
              </h2>
              <p className="text-xl mb-8 opacity-90">
                Будьте среди первых, кто протестирует революционный подход к подготовке ML-интервью с AI-интервьюером
              </p>
              
              <div className="flex flex-col sm:flex-row gap-4 justify-center items-center mb-8">
                <Link 
                  to="/auth" 
                  className="px-8 py-4 rounded-xl bg-white text-orange-600 font-bold hover:bg-orange-50 transition-all duration-300 shadow-lg hover:shadow-xl"
                >
                  Начать бесплатно
                </Link>
                <div className="text-sm opacity-80">
                  • Бесплатный период • Бета-тестирование • Ранний доступ к AI-интервьюеру
                </div>
              </div>
              
              <div className="grid grid-cols-3 gap-8 max-w-md mx-auto">
                <div className="text-center">
                  <div className="text-2xl font-bold">MVP</div>
                  <div className="text-sm opacity-80">Бета-версия</div>
                </div>
                <div className="text-center">
                  <div className="text-2xl font-bold">AI</div>
                  <div className="text-sm opacity-80">Интервьюер</div>
                </div>
                <div className="text-center">
                  <div className="text-2xl font-bold">2025</div>
                  <div className="text-sm opacity-80">Запуск</div>
                </div>
              </div>
            </div>
            
            {/* Decorative Elements */}
            <div className="absolute top-0 right-0 w-64 h-64 bg-white/10 rounded-full blur-3xl"></div>
            <div className="absolute bottom-0 left-0 w-48 h-48 bg-white/10 rounded-full blur-3xl"></div>
            <div className="absolute top-1/2 left-1/4 w-32 h-32 bg-white/5 rounded-full blur-2xl"></div>
          </div>
        </section>
      </main>

      <footer className="py-10 border-t border-orange-100">
        <div className="max-w-6xl mx-auto px-4 text-center">
          <div className="text-zinc-500 mb-2">
            AI-симулятор технических ML-интервью • Революционный подход к подготовке и оценке
          </div>
          <div className="text-sm text-zinc-400">© 2025 TensorTalks</div>
          <div className="mt-2 text-sm">
            <Link to="/privacy-policy" className="text-orange-600 hover:underline">Политика конфиденциальности</Link>
          </div>
        </div>
      </footer>

      {/* Scroll Indicator - Fixed Bottom Right */}
      <div 
        className={`fixed bottom-8 right-8 z-50 transition-opacity duration-700 ${showScrollIndicator ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
      >
        <div className="flex flex-col items-center gap-2 px-4 py-3 bg-white rounded-2xl shadow-lg border border-orange-200 animate-gentle-pulse">
          <span className="text-xs font-medium text-orange-600">Прокрутите</span>
          <div className="w-6 h-10 rounded-full border-2 border-orange-600 flex items-start justify-center p-2">
            <div className="w-1.5 h-2 bg-orange-600 rounded-full animate-bounce"></div>
          </div>
        </div>
        <style>{`
          @keyframes gentle-pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.7; }
          }
          .animate-gentle-pulse {
            animation: gentle-pulse 3s ease-in-out infinite;
          }
        `}</style>
      </div>
    </div>
  )
}
