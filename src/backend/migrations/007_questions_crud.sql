-- 007_questions_crud.sql
-- questions-crud-service tables.
-- Includes pgvector for semantic question search (§10.3).

CREATE EXTENSION IF NOT EXISTS vector;

-- Main questions table
CREATE TABLE IF NOT EXISTS questions_crud.questions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_text   TEXT NOT NULL,
    ideal_answer    TEXT,
    topic           VARCHAR(100) NOT NULL,
    subtopic        VARCHAR(100),
    difficulty      VARCHAR(20) NOT NULL DEFAULT 'middle',  -- junior, middle, senior
    question_type   VARCHAR(20) NOT NULL DEFAULT 'theory',  -- theory, practice, case
    tags            TEXT[] DEFAULT '{}',
    language        VARCHAR(10) DEFAULT 'ru',
    status          VARCHAR(20) NOT NULL DEFAULT 'published',
    -- Embeddings for semantic search (§10.3)
    embedding           vector(1536),       -- question_text embedding
    ideal_answer_embedding vector(1536),    -- ideal_answer embedding
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ivfflat index for semantic search
CREATE INDEX IF NOT EXISTS idx_questions_embedding
    ON questions_crud.questions
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Standard filter indexes
CREATE INDEX IF NOT EXISTS idx_questions_topic
    ON questions_crud.questions(topic, difficulty);
CREATE INDEX IF NOT EXISTS idx_questions_status
    ON questions_crud.questions(status);
CREATE INDEX IF NOT EXISTS idx_questions_tags
    ON questions_crud.questions USING GIN(tags);
