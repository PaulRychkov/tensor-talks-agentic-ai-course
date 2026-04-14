const BASE = '/admin'

export function getToken(): string | null {
  return localStorage.getItem('admin_token')
}

export function setToken(t: string) {
  localStorage.setItem('admin_token', t)
}

export function clearToken() {
  localStorage.removeItem('admin_token')
}

function authHeaders(): Record<string, string> {
  const t = getToken()
  return t ? { Authorization: `Bearer ${t}` } : {}
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error((err as { error?: string }).error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

export async function login(secret: string): Promise<string> {
  const data = await request<{ token: string }>('POST', '/login', { secret })
  return data.token
}

// ── Questions ──────────────────────────────────────────────────────────────────

export interface Question {
  id: string
  topic: string
  question_type: string
  complexity: number
  question_text: string
  ideal_answer?: string
  expected_points?: string[]
}

interface RawQuestion {
  ID: string
  TheoryID: string
  QuestionType: string
  Complexity: number
  Data: {
    content?: {
      question?: string
      expected_points?: string[]
    }
    ideal_answer?: { text?: string }
    topic?: string
  }
}

function mapQuestion(r: RawQuestion): Question {
  return {
    id: r.ID,
    topic: r.Data.topic ?? r.TheoryID ?? '',
    question_type: r.QuestionType,
    complexity: r.Complexity,
    question_text: r.Data.content?.question ?? r.ID,
    ideal_answer: r.Data.ideal_answer?.text,
    expected_points: r.Data.content?.expected_points,
  }
}

export async function searchQuestions(q?: string, topic?: string): Promise<Question[]> {
  const params = new URLSearchParams()
  if (q) params.set('q', q)
  if (topic) params.set('topic', topic)
  const qs = params.toString()
  const data = await request<{ questions?: RawQuestion[] }>('GET', `/api/questions${qs ? `?${qs}` : ''}`)
  return (data.questions ?? []).map(mapQuestion)
}

export interface QuestionPayload {
  id?: string
  question_type: string
  complexity: number
  content: { question: string; expected_points?: string[] }
  ideal_answer: { text: string }
  metadata: { topic?: string; language?: string; created_by?: string }
}

export async function createQuestion(payload: QuestionPayload): Promise<void> {
  await request('POST', '/api/questions', payload)
}

export async function updateQuestion(id: string, payload: QuestionPayload): Promise<void> {
  await request('PUT', `/api/questions/${id}`, payload)
}

export async function deleteQuestion(id: string): Promise<void> {
  await request('DELETE', `/api/questions/${id}`)
}

// ── Knowledge base ─────────────────────────────────────────────────────────────

export interface KnowledgeItem {
  id: string
  concept: string
  complexity: number
  tags: string[]
  summary: string
  segments?: { type: string; content: string }[]
}

interface RawKnowledge {
  ID: string
  Concept: string
  Complexity: number
  Tags: string[]
  Data: {
    segments?: { type: string; content: string }[]
  }
}

function mapKnowledge(r: RawKnowledge): KnowledgeItem {
  const definition = r.Data.segments?.find(s => s.type === 'definition')
  return {
    id: r.ID,
    concept: r.Concept,
    complexity: r.Complexity,
    tags: r.Tags ?? [],
    summary: definition?.content ?? '',
    segments: r.Data.segments,
  }
}

export async function searchKnowledge(q?: string, topic?: string): Promise<KnowledgeItem[]> {
  const params = new URLSearchParams()
  if (q) params.set('q', q)
  if (topic) params.set('topic', topic)
  const qs = params.toString()
  const data = await request<{ knowledge?: RawKnowledge[] }>('GET', `/api/knowledge${qs ? `?${qs}` : ''}`)
  return (data.knowledge ?? []).map(mapKnowledge)
}

export async function getKnowledgeItem(id: string): Promise<KnowledgeItem> {
  const data = await request<RawKnowledge>('GET', `/api/knowledge/${id}`)
  return mapKnowledge(data)
}

export interface KnowledgePayload {
  concept: string
  complexity: number
  tags: string[]
  data: {
    segments: { type: string; content: string }[]
  }
}

export async function updateKnowledgeItem(id: string, payload: KnowledgePayload): Promise<void> {
  await request('PUT', `/api/knowledge/${id}`, payload)
}

export async function deleteKnowledgeItem(id: string): Promise<void> {
  await request('DELETE', `/api/knowledge/${id}`)
}

// ── Metrics ────────────────────────────────────────────────────────────────────

export interface ProductMetrics {
  total_sessions: number
  completed_naturally: number
  terminated_early: number
  completion_rate: number
  avg_score: number
  avg_rating: number
  rated_sessions: number
  by_kind: Record<string, number>
}

export async function getProductMetrics(): Promise<ProductMetrics> {
  const data = await request<{ metrics: ProductMetrics }>('GET', '/api/metrics/product')
  return data.metrics
}

export async function getTechnicalMetrics(): Promise<Record<string, unknown>> {
  const data = await request<{ metrics: Record<string, unknown> }>('GET', '/api/metrics/technical')
  return data.metrics
}

export async function getAIMetrics(): Promise<Record<string, unknown>> {
  const data = await request<{ metrics: Record<string, unknown> }>('GET', '/api/metrics/ai')
  return data.metrics
}

// ── Upload ─────────────────────────────────────────────────────────────────────

export interface UploadResult {
  task_id?: string
  draft_id?: string
  status?: string
  message?: string
}

export async function uploadUrl(url: string, topic?: string): Promise<UploadResult> {
  return request<UploadResult>('POST', '/api/knowledge/upload', { url, topic })
}

export async function uploadFile(file: File, topic?: string): Promise<UploadResult> {
  const t = getToken()
  const formData = new FormData()
  formData.append('file', file)
  const params = new URLSearchParams()
  if (topic) params.set('topic', topic)
  const qs = params.toString()
  const res = await fetch(`${BASE}/api/knowledge/upload${qs ? `?${qs}` : ''}`, {
    method: 'POST',
    headers: t ? { Authorization: `Bearer ${t}` } : {},
    body: formData,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error((err as { error?: string }).error ?? res.statusText)
  }
  return res.json() as Promise<UploadResult>
}

// ── Drafts ─────────────────────────────────────────────────────────────────────

export interface Draft {
  id: string
  type: string
  topic?: string
  title?: string
  status: string
  created_at: string
  duplicate_candidate?: boolean
  content?: string
}

export async function getDrafts(status?: string): Promise<Draft[]> {
  const qs = status ? `?status=${status}` : ''
  const data = await request<{ drafts?: Draft[] } | Draft[]>('GET', `/api/drafts${qs}`)
  if (Array.isArray(data)) return data
  return (data as { drafts?: Draft[] }).drafts ?? []
}

export async function getDraft(id: string): Promise<Draft> {
  return request<Draft>('GET', `/api/drafts/${id}`)
}

export async function approveDraft(id: string): Promise<void> {
  await request('POST', `/api/drafts/${id}/approve`)
}

export async function rejectDraft(id: string, comment?: string): Promise<void> {
  await request('POST', `/api/drafts/${id}/reject`, { comment })
}

// ── Login words ────────────────────────────────────────────────────────────────

export interface LoginWord {
  id: number
  word: string
}

export async function getAdjectives(): Promise<LoginWord[]> {
  const data = await request<LoginWord[] | { adjectives?: LoginWord[] }>('GET', '/api/login-words/adjectives')
  return Array.isArray(data) ? data : (data.adjectives ?? [])
}

export async function createAdjective(word: string): Promise<LoginWord> {
  return request<LoginWord>('POST', '/api/login-words/adjectives', { word })
}

export async function updateAdjective(id: number, word: string): Promise<LoginWord> {
  return request<LoginWord>('PUT', `/api/login-words/adjectives/${id}`, { word })
}

export async function deleteAdjective(id: number): Promise<void> {
  await request('DELETE', `/api/login-words/adjectives/${id}`)
}

export async function getNouns(): Promise<LoginWord[]> {
  const data = await request<LoginWord[] | { nouns?: LoginWord[] }>('GET', '/api/login-words/nouns')
  return Array.isArray(data) ? data : (data.nouns ?? [])
}

export async function createNoun(word: string): Promise<LoginWord> {
  return request<LoginWord>('POST', '/api/login-words/nouns', { word })
}

export async function updateNoun(id: number, word: string): Promise<LoginWord> {
  return request<LoginWord>('PUT', `/api/login-words/nouns/${id}`, { word })
}

export async function deleteNoun(id: number): Promise<void> {
  await request('DELETE', `/api/login-words/nouns/${id}`)
}
