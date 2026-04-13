const apiBaseEnv = import.meta.env.VITE_API_BASE_URL ?? '/api';
const API_BASE = apiBaseEnv.replace(/\/$/, '');

type AuthResponse = {
  user: {
    id: string;
    login: string;
  };
  tokens: {
    access_token: string;
    refresh_token: string;
  };
};

async function request<T>(path: string, options: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
  });

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    // Используем детальное сообщение об ошибке из backend, если оно есть
    let message = data?.error ?? 'Произошла ошибка. Попробуйте еще раз.';
    
    // Улучшаем сообщения для пользователя
    if (message === 'internal error') {
      message = 'Внутренняя ошибка сервера. Пожалуйста, попробуйте позже.';
    } else if (message === 'login already exists') {
      message = 'Пользователь с таким логином уже существует.';
    } else if (message === 'invalid credentials') {
      message = 'Неверный логин или пароль.';
    } else if (message === 'invalid payload') {
      message = 'Некорректные данные. Проверьте введенные значения.';
    } else if (message === 'invalid token' || message === 'missing token') {
      // Очищаем localStorage при невалидном токене
      localStorage.removeItem('tt_tokens');
      localStorage.removeItem('tt_user');
      message = 'Сессия истекла. Пожалуйста, войдите снова.';
    }
    
    throw new Error(message);
  }

  return (await response.json()) as T;
}

export async function register(login: string, password: string): Promise<AuthResponse> {
  return request<AuthResponse>('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ login, password }),
  });
}

export async function login(login: string, password: string): Promise<AuthResponse> {
  return request<AuthResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ login, password }),
  });
}

