import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Chat from '../Chat'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

vi.mock('../../services/chat', () => ({
  sendMessage: vi.fn(),
  getNextQuestion: vi.fn().mockResolvedValue(null),
  getResults: vi.fn().mockResolvedValue(null),
  resumeChat: vi.fn().mockResolvedValue(undefined),
  getInterviewChat: vi.fn().mockResolvedValue([]),
  terminateChat: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../../components/TerminateConfirmModal', () => ({
  default: ({ isOpen }: { isOpen: boolean }) =>
    isOpen ? <div data-testid="terminate-modal">Terminate?</div> : null,
}))

function renderChat(sessionId = 'abc-123') {
  return render(
    <MemoryRouter initialEntries={[`/chat/${sessionId}`]}>
      <Routes>
        <Route path="/chat/:id" element={<Chat />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Chat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.setItem('tt_user', JSON.stringify({ id: 'user-1', login: 'tester' }))
  })

  it('renders without crashing and shows session id', () => {
    renderChat()
    expect(screen.getByText(/abc-123/)).toBeInTheDocument()
  })

  it('shows the waiting message when no messages are present', () => {
    renderChat()
    expect(
      screen.getByText(/Ожидайте первого вопроса/),
    ).toBeInTheDocument()
  })

  it('renders a question message with ReactMarkdown', async () => {
    const { getInterviewChat } = await import('../../services/chat')
    vi.mocked(getInterviewChat).mockResolvedValueOnce([
      { type: 'system', content: 'Вопрос: **Bold question?**', created_at: '2025-01-01T00:00:00Z' },
    ])

    renderChat('sess-md')
    const bold = await screen.findByText('Bold question?')
    expect(bold.tagName).toBe('STRONG')
  })

  it('renders user messages from history', async () => {
    const { getInterviewChat } = await import('../../services/chat')
    vi.mocked(getInterviewChat).mockResolvedValueOnce([
      { type: 'user', content: 'My answer', created_at: '2025-01-01T00:01:00Z' },
    ])

    renderChat('sess-user')
    expect(await screen.findByText('My answer')).toBeInTheDocument()
  })

  it('renders hint messages from history', async () => {
    const { getInterviewChat } = await import('../../services/chat')
    vi.mocked(getInterviewChat).mockResolvedValueOnce([
      { type: 'system', content: 'Подсказка: Think about gradients', created_at: '2025-01-01T00:02:00Z' },
    ])

    renderChat('sess-hint')
    expect(await screen.findByText('Think about gradients')).toBeInTheDocument()
    expect(screen.getByText('Подсказка')).toBeInTheDocument()
  })

  it('renders summary messages from history', async () => {
    const { getInterviewChat } = await import('../../services/chat')
    vi.mocked(getInterviewChat).mockResolvedValueOnce([
      { type: 'system', content: 'Summary: You covered basics', created_at: '2025-01-01T00:03:00Z' },
    ])

    renderChat('sess-summary')
    expect(await screen.findByText('You covered basics')).toBeInTheDocument()
    expect(screen.getByText('Промежуточная сводка')).toBeInTheDocument()
  })

  it('renders plain system messages from history', async () => {
    const { getInterviewChat } = await import('../../services/chat')
    vi.mocked(getInterviewChat).mockResolvedValueOnce([
      { type: 'system', content: 'Some system note', created_at: '2025-01-01T00:04:00Z' },
    ])

    renderChat('sess-sys')
    expect(await screen.findByText('Some system note')).toBeInTheDocument()
    expect(screen.getByText('Интервьюер')).toBeInTheDocument()
  })

  it('redirects to /auth when no user in localStorage', () => {
    localStorage.removeItem('tt_user')
    renderChat()
    expect(mockNavigate).toHaveBeenCalledWith('/auth')
  })
})
