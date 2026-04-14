-- 005_results_crud.sql
-- results-crud-service tables.
-- Stores session results, evaluations, training presets, and topic progress.

-- Session results
CREATE TABLE IF NOT EXISTS results_crud.results (
    id                      BIGSERIAL PRIMARY KEY,
    session_id              UUID NOT NULL UNIQUE,
    user_id                 UUID,
    score                   INT NOT NULL,
    feedback                TEXT NOT NULL,
    terminated_early        BOOLEAN DEFAULT FALSE,
    report_json             JSONB,               -- full analyst report
    preset_training         JSONB,               -- training recommendations
    evaluations             JSONB,               -- per-question evaluations array
    session_kind            VARCHAR(20) DEFAULT 'interview',  -- interview, training, study
    result_format_version   INT DEFAULT 1,
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_results_user_id
    ON results_crud.results(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_results_session_id
    ON results_crud.results(session_id);

-- Training presets (analyst-generated follow-up session configs)
CREATE TABLE IF NOT EXISTS results_crud.presets (
    preset_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL,
    target_mode         VARCHAR(20) NOT NULL,  -- 'training', 'study'
    topics              TEXT[],
    materials           TEXT[],
    source_session_id   UUID,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at          TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_presets_user_id
    ON results_crud.presets(user_id, created_at DESC);

-- Per-user per-topic theory completion progress
CREATE TABLE IF NOT EXISTS results_crud.user_topic_progress (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             UUID NOT NULL,
    topic_id            VARCHAR(100) NOT NULL,
    theory_completed_at TIMESTAMP WITH TIME ZONE,
    source_session_id   UUID,
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (user_id, topic_id)
);

CREATE INDEX IF NOT EXISTS idx_topic_progress_user
    ON results_crud.user_topic_progress(user_id);
