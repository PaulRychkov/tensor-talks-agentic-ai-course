import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Results from '../Results'
import type { ReportJSON, QuestionEvaluation, InterviewResult, ResultsResponse } from '../../services/chat'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockGetResults = vi.fn<() => Promise<ResultsResponse | null>>()
const mockGetInterviewResult = vi.fn<() => Promise<InterviewResult | null>>()
const mockGetInterviewChat = vi.fn()

vi.mock('../../services/chat', () => ({
  getResults: () => mockGetResults(),
  getInterviewResult: () => mockGetInterviewResult(),
  getInterviewChat: () => mockGetInterviewChat(),
}))

function renderResults(sessionId = 'result-123') {
  return render(
    <MemoryRouter initialEntries={[`/results/${sessionId}`]}>
      <Routes>
        <Route path="/results/:id" element={<Results />} />
      </Routes>
    </MemoryRouter>,
  )
}

const baseResult: ResultsResponse = {
  score: 75,
  feedback: 'Good job overall',
  recommendations: ['Study regularization', 'Practice SQL'],
  completed_at: '2025-01-15T12:00:00Z',
}

describe('Results', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetResults.mockResolvedValue(null)
    mockGetInterviewResult.mockResolvedValue(null)
    mockGetInterviewChat.mockResolvedValue([])
  })

  describe('ScoreBadge', () => {
    it('renders green badge for score >= 80', async () => {
      mockGetResults.mockResolvedValueOnce({ ...baseResult, score: 92 })
      renderResults()

      const badge = await screen.findByText('92%')
      expect(badge).toBeInTheDocument()
      expect(badge.className).toContain('text-green-700')
    })

    it('renders yellow badge for score 50-79', async () => {
      mockGetResults.mockResolvedValueOnce({ ...baseResult, score: 65 })
      renderResults()

      const badge = await screen.findByText('65%')
      expect(badge).toBeInTheDocument()
      expect(badge.className).toContain('text-yellow-700')
    })

    it('renders red badge for score < 50', async () => {
      mockGetResults.mockResolvedValueOnce({ ...baseResult, score: 30 })
      renderResults()

      const badge = await screen.findByText('30%')
      expect(badge).toBeInTheDocument()
      expect(badge.className).toContain('text-red-700')
    })
  })

  describe('partial report_json', () => {
    it('renders only summary when other sections are missing', async () => {
      const report: ReportJSON = {
        summary: 'Candidate showed basic knowledge',
        errors_by_topic: {},
        strengths: [],
        preparation_plan: [],
        materials: [],
      }

      mockGetResults.mockResolvedValueOnce(baseResult)
      mockGetInterviewResult.mockResolvedValueOnce({
        id: 1,
        session_id: 'result-123',
        score: 75,
        feedback: 'Good',
        terminated_early: false,
        created_at: '2025-01-15T12:00:00Z',
        updated_at: '2025-01-15T12:00:00Z',
        report_json: report,
      })

      renderResults()

      expect(await screen.findByText('Резюме')).toBeInTheDocument()
      expect(screen.getByText('Candidate showed basic knowledge')).toBeInTheDocument()
      expect(screen.queryByText('Сильные стороны')).not.toBeInTheDocument()
      expect(screen.queryByText('Ошибки по темам')).not.toBeInTheDocument()
      expect(screen.queryByText('План подготовки')).not.toBeInTheDocument()
    })

    it('renders fallback feedback when report_json is absent', async () => {
      mockGetResults.mockResolvedValueOnce(baseResult)
      mockGetInterviewResult.mockResolvedValueOnce({
        id: 1,
        session_id: 'result-123',
        score: 75,
        feedback: 'Good job overall',
        terminated_early: false,
        created_at: '2025-01-15T12:00:00Z',
        updated_at: '2025-01-15T12:00:00Z',
      })

      renderResults()

      expect(await screen.findByText('Good job overall')).toBeInTheDocument()
      expect(screen.queryByText('Резюме')).not.toBeInTheDocument()
    })
  })

  describe('question evaluations', () => {
    it('renders evaluation table with scores', async () => {
      const evaluations: QuestionEvaluation[] = [
        { question_id: 'q1', score: 8, decision: 'correct', topic: 'ML basics' },
        { question_id: 'q2', score: 3, decision: 'incorrect', topic: 'Neural nets' },
      ]

      mockGetResults.mockResolvedValueOnce(baseResult)
      mockGetInterviewResult.mockResolvedValueOnce({
        id: 1,
        session_id: 'result-123',
        score: 55,
        feedback: 'Mixed',
        terminated_early: false,
        created_at: '2025-01-15T12:00:00Z',
        updated_at: '2025-01-15T12:00:00Z',
        evaluations,
      })

      renderResults()

      expect(await screen.findByText('Оценки по вопросам')).toBeInTheDocument()
      expect(screen.getByText('ML basics')).toBeInTheDocument()
      expect(screen.getByText('Neural nets')).toBeInTheDocument()
      expect(screen.getByText('8/10')).toBeInTheDocument()
      expect(screen.getByText('3/10')).toBeInTheDocument()

      const highScore = screen.getByText('8/10')
      expect(highScore.className).toContain('text-green-700')

      const lowScore = screen.getByText('3/10')
      expect(lowScore.className).toContain('text-red-700')
    })
  })

  it('shows "no results" when nothing is returned', async () => {
    renderResults()
    expect(await screen.findByText('Результаты не найдены')).toBeInTheDocument()
  })

  it('shows chat history when messages exist', async () => {
    mockGetResults.mockResolvedValueOnce(baseResult)
    mockGetInterviewChat.mockResolvedValueOnce([
      { type: 'system', content: 'What is overfitting?', created_at: '2025-01-15T12:00:00Z' },
      { type: 'user', content: 'It is when model memorizes', created_at: '2025-01-15T12:01:00Z' },
    ])

    renderResults()

    expect(await screen.findByText('What is overfitting?')).toBeInTheDocument()
    expect(screen.getByText('It is when model memorizes')).toBeInTheDocument()
  })
})
