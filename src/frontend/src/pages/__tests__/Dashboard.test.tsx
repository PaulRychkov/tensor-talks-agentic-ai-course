import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Dashboard from '../Dashboard'
import type { InterviewInfo } from '../../services/chat'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockGetInterviews = vi.fn<() => Promise<InterviewInfo[]>>()
const mockStartChat = vi.fn()

vi.mock('../../services/chat', () => ({
  getInterviews: () => mockGetInterviews(),
  startChat: () => mockStartChat(),
}))

vi.mock('../../components/MVPNotification', () => ({
  default: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="mvp-popup">MVP</div> : null,
}))

vi.mock('../../components/InterviewParamsModal', () => ({
  default: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="params-modal">Params</div> : null,
}))

function renderDashboard() {
  return render(
    <MemoryRouter>
      <Dashboard />
    </MemoryRouter>,
  )
}

const makeInterview = (overrides: Partial<InterviewInfo> = {}): InterviewInfo => ({
  session_id: 'sess-1',
  start_time: '2025-06-01T10:00:00Z',
  params: { topics: ['classic_ml'], level: 'middle', mode: 'interview' },
  has_results: true,
  score: 82,
  feedback: 'Great performance',
  ...overrides,
})

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.setItem('tt_user', JSON.stringify({ id: 'user-1', login: 'tester' }))
    mockGetInterviews.mockResolvedValue([])
  })

  it('renders the dashboard header and title', () => {
    renderDashboard()
    expect(screen.getByText('Дашборд')).toBeInTheDocument()
    expect(screen.getByText('TensorTalks')).toBeInTheDocument()
  })

  it('shows the user login in the header', () => {
    renderDashboard()
    expect(screen.getByText('tester')).toBeInTheDocument()
  })

  it('renders session list from API', async () => {
    const interviews: InterviewInfo[] = [
      makeInterview({ session_id: 's1', score: 90, feedback: 'Excellent' }),
      makeInterview({
        session_id: 's2',
        params: { topics: ['nlp'], level: 'senior', mode: 'training' },
        score: 45,
        feedback: 'Needs work',
      }),
    ]
    mockGetInterviews.mockResolvedValueOnce(interviews)

    renderDashboard()

    expect(await screen.findByText('90%')).toBeInTheDocument()
    expect(screen.getByText('45%')).toBeInTheDocument()
    expect(screen.getByText('classic_ml')).toBeInTheDocument()
    expect(screen.getByText('nlp')).toBeInTheDocument()
  })

  it('shows empty state when no interviews exist', async () => {
    mockGetInterviews.mockResolvedValueOnce([])
    renderDashboard()
    expect(await screen.findByText('Нет пройденных сессий')).toBeInTheDocument()
  })

  it('shows training mode label for training sessions', async () => {
    mockGetInterviews.mockResolvedValueOnce([
      makeInterview({
        session_id: 's-train',
        params: { topics: ['llm'], level: 'junior', mode: 'training' },
      }),
    ])

    renderDashboard()
    expect(await screen.findByText('Тренировка')).toBeInTheDocument()
  })

  it('shows active status for sessions without end_time', async () => {
    mockGetInterviews.mockResolvedValueOnce([
      makeInterview({
        session_id: 's-active',
        end_time: undefined,
        has_results: false,
        score: null,
      }),
    ])

    renderDashboard()
    expect(await screen.findByText('Активная')).toBeInTheDocument()
  })

  it('renders quick-action buttons', () => {
    renderDashboard()
    expect(screen.getByText('Начать интервью')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Тренировка' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Изучение темы' })).toBeInTheDocument()
  })
})
