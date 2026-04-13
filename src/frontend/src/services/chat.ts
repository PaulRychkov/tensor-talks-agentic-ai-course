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
    
    // Для остальных ошибок - обычная обработка
    const error = new Error(message);
    if (response.status === 404) {
      (error as any).status = 404;
    }
    throw error;
  }

  return (await response.json()) as T;
}

export interface SessionParams {
  topics: string[]; // classic_ml, nlp, llm
  level: string; // junior, middle, senior
  type: string;  // interview, training
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
}

export async function getNextQuestion(sessionId: string): Promise<QuestionResponse | null> {
  try {
    const response = await request<any>(`/chat/${sessionId}/question`, {
      method: 'GET',
    });
    // Проверяем новый формат (200 с available: false)
    if (response.available === false || response.question === null) {
      return null;
    }
    // Проверяем старый формат (прямой QuestionResponse)
    if (response.question && response.question_id && response.timestamp) {
      return response as QuestionResponse;
    }
    return null;
  } catch (error: any) {
    // Обрабатываем старый формат (404) для обратной совместимости
    if (error.message?.includes('no new questions') || error.message?.includes('404') || error.message?.includes('chat not completed')) {
      return null;
    }
    // Только реальные ошибки логируем
    console.error('getNextQuestion error:', error);
    throw error;
  }
}

export interface ResultsResponse {
  score: number;
  feedback: string;
  recommendations: string[];
  completed_at: string;
}

export async function getResults(sessionId: string): Promise<ResultsResponse | null> {
  try {
    const response = await request<any>(`/chat/${sessionId}/results`, {
      method: 'GET',
    });
    // Проверяем новый формат (200 с available: false)
    if (response.available === false || response.results === null) {
      return null;
    }
    // Проверяем старый формат (прямой ResultsResponse)
    if (response.score !== undefined && response.feedback && response.recommendations) {
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

export interface InterviewResult {
  id: number;
  session_id: string;
  score: number;
  feedback: string;
  terminated_early: boolean;
  created_at: string;
  updated_at: string;
}

export interface InterviewResultResponse {
  result: InterviewResult;
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
