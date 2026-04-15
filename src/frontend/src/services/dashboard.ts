/**
 * Dashboard API client (§10.2).
 * Fetches real data from bff-service /api/dashboard/* endpoints.
 */

const apiBaseEnv = import.meta.env.VITE_API_BASE_URL ?? '/api';
const API_BASE = apiBaseEnv.replace(/\/$/, '');

function getAuthHeaders(): HeadersInit {
  try {
    const tokens = JSON.parse(localStorage.getItem('tt_tokens') ?? '{}');
    const accessToken = tokens?.access_token;
    if (accessToken) {
      return { 'Authorization': `Bearer ${accessToken}` };
    }
  } catch {}
  return {};
}

async function authRequest<T>(path: string, method: string = 'GET'): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
  });

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.error ?? `Request failed: ${response.status}`);
  }

  return (await response.json()) as T;
}

export type DashboardSummary = {
  total_sessions: number;
  completed_sessions: number;
  avg_score: number;
  streak_days: number;
  current_level: string;
  training_unlocked_topics: string[];
  last_session_date: string | null;
};

export type ActivityEntry = {
  date: string;   // ISO date string YYYY-MM-DD
  count: number;
};

export type TopicProgress = {
  topic: string;
  score: number;  // 0.0-1.0
  status: 'not_started' | 'in_progress' | 'completed';
  sessions: number;
};

export type DashboardRecommendation = {
  topic: string;
  action: string;
  priority: 'high' | 'medium' | 'low';
};

/**
 * Fetch dashboard summary metrics.
 * Returns null on network error (caller shows empty state, NOT hardcoded values).
 */
export async function getDashboardSummary(): Promise<DashboardSummary | null> {
  try {
    return await authRequest<DashboardSummary>('/dashboard/summary');
  } catch (err) {
    console.error('[dashboard] summary fetch failed', err);
    return null;
  }
}

/**
 * Fetch calendar activity data for a date range.
 */
export async function getDashboardActivity(
  from?: string,
  to?: string,
): Promise<ActivityEntry[]> {
  try {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return await authRequest<ActivityEntry[]>(`/dashboard/activity${qs}`);
  } catch (err) {
    console.error('[dashboard] activity fetch failed', err);
    return [];
  }
}

/**
 * Fetch per-topic progress data.
 */
export async function getDashboardTopicProgress(): Promise<TopicProgress[]> {
  try {
    return await authRequest<TopicProgress[]>('/dashboard/topic-progress');
  } catch (err) {
    console.error('[dashboard] topic-progress fetch failed', err);
    return [];
  }
}

/**
 * Fetch recommendations from the last report.
 */
export async function getDashboardRecommendations(): Promise<DashboardRecommendation[]> {
  try {
    return await authRequest<DashboardRecommendation[]>('/dashboard/recommendations');
  } catch (err) {
    console.error('[dashboard] recommendations fetch failed', err);
    return [];
  }
}

export type SubtopicEntry = {
  id: string;
  label: string;
  topics: string[];  // e.g. ["llm", "nlp", "classic_ml"]
};

export type PresetItem = {
  preset_id: string;
  user_id: string;
  target_mode: 'study' | 'training';
  topics: string[];
  materials: string[];            // entries like "focus_point:<title>" or subtopic ids
  source_session_id?: string;
  created_at: string;
  expires_at?: string;
};

/**
 * Fetch presets (study/training follow-ups) for the current user.
 */
export async function getUserPresets(): Promise<PresetItem[]> {
  try {
    const resp = await authRequest<{ presets: PresetItem[] }>('/presets');
    return resp.presets ?? [];
  } catch (err) {
    console.error('[dashboard] presets fetch failed', err);
    return [];
  }
}

/**
 * Delete a preset after it has been used to start a session.
 */
export async function deletePreset(presetId: string): Promise<void> {
  try {
    await authRequest<{ deleted: boolean }>(`/presets/${presetId}`, 'DELETE');
  } catch (err) {
    console.error('[dashboard] preset delete failed', err);
  }
}

/**
 * Fetch available subtopics from knowledge base.
 * Public endpoint — no auth required.
 */
export async function getSubtopics(): Promise<SubtopicEntry[]> {
  try {
    const resp = await fetch(`${API_BASE}/subtopics`);
    if (!resp.ok) return [];
    const data = await resp.json();
    return data.subtopics ?? [];
  } catch (err) {
    console.error('[dashboard] subtopics fetch failed', err);
    return [];
  }
}
