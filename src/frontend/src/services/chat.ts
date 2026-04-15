const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api';

async function request<T>(path: string, options: RequestInit): Promise<T> {
  const tokens = localStorage.getItem('tt_tokens');
  const token = tokens ? JSON.parse(tokens).access_token : null;

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers ?? {}),
    },
  });

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    const message = data?.error ?? 'Произошла ошибка. Попробуйте еще раз.';
    
    // Если токен невалиден, очищаем localStorage и редиректим на страницу логина
    if (response.status === 401 && (message === 'invalid token' || message === 'missing token')) {
      localStorage.removeItem('tt_tokens');
      localStorage.removeItem('tt_user');
      // Используем window.location для надежного редиректа
      if (window.location.pathname !== '/auth') {
        window.location.href = '/auth';
      }
    }
    
    // Для 404 ошибок на polling endpoints - подавляем логирование в консоль браузера
    if (response.status === 404 && (path.includes('/question') || path.includes('/results'))) {
      // Создаем тихую ошибку без логирования в консоль браузера
      const error = new Error(message);
      (error as any).status = 404;
      (error as any).silent = true; // Флаг для подавления логирования
      throw error;
    }
    
    // 422 — PII detected (передаём детали для отображения предупреждения)
    if (response.status === 422 && data?.pii_detected) {
      const error = new Error(data.error ?? message);
      (error as any).status = 422;
      (error as any).pii_detected = true;
      (error as any).error = data.error;
      throw error;
    }

    // Для остальных ошибок - обычная обработка
    const error = new Error(message);
    if (response.status === 404) {
      (error as any).status = 404;
    }
    throw error;
  }

  return (await response.json()) as T;
}

export type SessionMode = 'interview' | 'training' | 'study';

export interface SessionParams {
  topics: string[];
  level: string;
  mode: SessionMode;
  type?: string;
  source?: 'manual' | 'preset';
  preset_id?: string;
  subtopics?: string[];
  use_previous_results?: boolean;
  num_questions?: number;
  focus_points?: string[];
}

export interface StartChatResponse {
  session_id: string;
  ready: boolean;
}

export async function startChat(userId: string, params: SessionParams): Promise<StartChatResponse> {
  try {
    const response = await request<StartChatResponse>('/chat/start', {
      method: 'POST',
      body: JSON.stringify({ 
        user_id: userId,
        params: params
      }),
    });
    console.log('startChat response:', response);
    return response;
  } catch (error) {
    console.error('startChat error:', error);
    throw error;
  }
}

export async function sendMessage(sessionId: string, content: string): Promise<void> {
  return request<void>('/chat/message', {
    method: 'POST',
    body: JSON.stringify({
      session_id: sessionId,
      content: content,
    }),
  });
}

export async function resumeChat(sessionId: string): Promise<void> {
  return request<void>(`/chat/${sessionId}/resume`, {
    method: 'POST',
    body: JSON.stringify({}),
  });
}

export async function terminateChat(sessionId: string): Promise<void> {
  return request<void>(`/chat/${sessionId}/terminate`, {
    method: 'POST',
    body: JSON.stringify({}),
  });
}

export interface QuestionResponse {
  question: string;
  question_id: string;
  timestamp: string;
  question_number: number;
  total_questions: number;
  pii_masked_content?: string;
}

export interface PollResult {
  question: QuestionResponse | null;
  processingStep: string; // '' when idle, 'processing' when agent is working
}

export async function pollQuestion(sessionId: string): Promise<PollResult> {
  try {
    const response = await request<any>(`/chat/${sessionId}/question`, {
      method: 'GET',
    });
    if (response.available === false || response.question === null) {
      return { question: null, processingStep: response.processing_step ?? '' };
    }
    if (response.question && response.question_id && response.timestamp) {
      return { question: response as QuestionResponse, processingStep: '' };
    }
    return { question: null, processingStep: '' };
  } catch (error: any) {
    if (error.message?.includes('no new questions') || error.message?.includes('404') || error.message?.includes('chat not completed')) {
      return { question: null, processingStep: '' };
    }
    console.error('pollQuestion error:', error);
    throw error;
  }
}

/** @deprecated use pollQuestion */
export async function getNextQuestion(sessionId: string): Promise<QuestionResponse | null> {
  const result = await pollQuestion(sessionId);
  return result.question;
}

export async function submitMessageFeedback(sessionId: string, questionId: string, rating: number): Promise<void> {
  try {
    await request<void>(`/chat/${sessionId}/message-feedback`, {
      method: 'POST',
      body: JSON.stringify({ question_id: questionId, rating }),
    });
  } catch {
    // non-critical, ignore errors
  }
}

export interface ResultsResponse {
  score: number;
  feedback: string;
  recommendations: string[];
  completed_at: string;
  /** True when the interview is done but the analyst report is still being generated */
  pending?: boolean;
}

export async function getResults(sessionId: string): Promise<ResultsResponse | null> {
  try {
    const response = await request<any>(`/chat/${sessionId}/results`, {
      method: 'GET',
    });
    // Interview completed, analyst still processing — return pending sentinel
    if (response.completed === true && response.pending === true) {
      return { score: 0, feedback: '', recommendations: [], completed_at: '', pending: true };
    }
    // Not completed yet
    if (response.available === false || response.results === null) {
      return null;
    }
    // Full results available
    if (response.score !== undefined && response.feedback !== undefined && response.recommendations) {
      return response as ResultsResponse;
    }
    return null;
  } catch (error: any) {
    // Обрабатываем старый формат (404) для обратной совместимости
    if (error.message?.includes('not completed') || error.message?.includes('404') || error.message?.includes('chat not completed')) {
      return null;
    }
    // Только реальные ошибки логируем
    console.error('getResults error:', error);
    throw error;
  }
}

export interface InterviewInfo {
  session_id: string;
  start_time: string;
  end_time?: string;
  params: SessionParams;
  has_results: boolean;
  score?: number | null;
  feedback?: string;
  terminated_early?: boolean;
}

export interface InterviewsResponse {
  interviews: InterviewInfo[];
}

export async function getInterviews(userId: string): Promise<InterviewInfo[]> {
  try {
    const response = await request<InterviewsResponse>(`/interviews?user_id=${userId}`, {
      method: 'GET',
    });
    return response.interviews;
  } catch (error) {
    console.error('getInterviews error:', error);
    throw error;
  }
}

export interface ChatMessage {
  type: string;
  content: string;
  created_at: string;
}

export interface ChatHistoryResponse {
  messages: ChatMessage[];
}

export async function getInterviewChat(sessionId: string): Promise<ChatMessage[]> {
  try {
    const response = await request<ChatHistoryResponse>(`/interviews/${sessionId}/chat`, {
      method: 'GET',
    });
    return response.messages;
  } catch (error) {
    console.error('getInterviewChat error:', error);
    throw error;
  }
}

export interface ErrorEntry {
  question: string;
  error: string;
  correction: string;
}

export interface StudyTheoryEntry {
  topic?: string;
  question: string;
  theory: string;
  order: number;
}

export interface ReportJSON {
  summary: string;
  errors_by_topic: Record<string, ErrorEntry[]>;
  strengths: string[];
  preparation_plan: string[];
  materials: string[];
  study_plan?: string[];
  theory_reviewed?: StudyTheoryEntry[];
  unmastered_topics?: string[];
}

export interface PresetTraining {
  weak_topics: string[];
  recommended_materials: string[];
  preset_id: string;
  follow_up_kind?: 'training' | 'study';
}

export interface QuestionEvaluation {
  question_id: string;
  score: number;
  decision: string;
  topic?: string;
}

export interface InterviewResult {
  id: number;
  session_id: string;
  score: number;
  feedback: string;
  terminated_early: boolean;
  created_at: string;
  updated_at: string;
  report_json?: ReportJSON;
  preset_training?: PresetTraining;
  evaluations?: QuestionEvaluation[];
  result_format_version?: number;
  session_kind?: SessionMode;
}

export interface InterviewResultResponse {
  result: InterviewResult;
}

export type ChatEventType =
  | 'chat.model_question'
  | 'chat.model_hint'
  | 'chat.model_summary'
  | 'chat.completed';

export interface ChatEventPayload {
  message: string;
  decision: string;
  question_index: number;
}

export interface ChatEvent {
  session_id: string;
  message_id: string;
  event_type: ChatEventType;
  timestamp: string;
  payload: ChatEventPayload;
}

export interface RecommendationItem {
  topic_id: string;
  title: string;
  level: string;
  eta_minutes: number;
  study_completed: boolean;
  training_unlocked: boolean;
}

export interface RecommendationsResponse {
  items: RecommendationItem[];
}

export async function getInterviewResult(sessionId: string): Promise<InterviewResult | null> {
  try {
    const response = await request<InterviewResultResponse>(`/interviews/${sessionId}/result`, {
      method: 'GET',
    });
    return response.result;
  } catch (error: any) {
    if (error.message?.includes('not found') || error.message?.includes('404')) {
      return null;
    }
    console.error('getInterviewResult error:', error);
    throw error;
  }
}

export async function submitSessionRating(sessionId: string, rating: number, comment: string): Promise<void> {
  await request<{ ok: boolean }>(`/interviews/${sessionId}/rating`, {
    method: 'POST',
    body: JSON.stringify({ rating, comment }),
  });
}
