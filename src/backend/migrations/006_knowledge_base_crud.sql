-- 006_knowledge_base_crud.sql
-- knowledge-base-crud-service tables.
-- Includes pgvector extension and embedding columns for semantic search (§10.3).

CREATE EXTENSION IF NOT EXISTS vector;

-- Main knowledge segments table
CREATE TABLE IF NOT EXISTS knowledge_base_crud.knowledge_segments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title           VARCHAR(255) NOT NULL,
    content         TEXT NOT NULL,
    topic           VARCHAR(100) NOT NULL,
    segment_type    VARCHAR(50) NOT NULL DEFAULT 'theory',  -- theory, example, definition
    tags            TEXT[] DEFAULT '{}',
    difficulty      VARCHAR(20) DEFAULT 'middle',
    language        VARCHAR(10) DEFAULT 'ru',
    source_url      TEXT,
    version         INT NOT NULL DEFAULT 1,
    status          VARCHAR(20) NOT NULL DEFAULT 'published',  -- draft, published, archived
    embedding       vector(1536),  -- text-embedding-3-small; §10.3
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for semantic search (ivfflat for scalability up to ~100K records)
CREATE INDEX IF NOT EXISTS idx_segments_embedding
    ON knowledge_base_crud.knowledge_segments
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Standard filter indexes
CREATE INDEX IF NOT EXISTS idx_segments_topic
    ON knowledge_base_crud.knowledge_segments(topic);
CREATE INDEX IF NOT EXISTS idx_segments_status
    ON knowledge_base_crud.knowledge_segments(status);
CREATE INDEX IF NOT EXISTS idx_segments_tags
    ON knowledge_base_crud.knowledge_segments USING GIN(tags);

-- Embedding model metadata table (§10.3)
CREATE TABLE IF NOT EXISTS knowledge_base_crud.embedding_metadata (
    id              SERIAL PRIMARY KEY,
    model_name      VARCHAR(100) NOT NULL,   -- 'text-embedding-3-small'
    model_version   VARCHAR(50) NOT NULL,    -- 'v1'
    dimension       INT NOT NULL,            -- 1536
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_current      BOOLEAN NOT NULL DEFAULT TRUE
);

-- Drafts table for knowledge producer HITL review
CREATE TABLE IF NOT EXISTS knowledge_base_crud.knowledge_drafts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content             JSONB NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
    source_url          TEXT,
    operator_id         BIGINT,
    operator_comment    TEXT,
    duplicate_candidate BOOLEAN NOT NULL DEFAULT FALSE,
    similar_segment_ids UUID[] DEFAULT '{}',
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drafts_status
    ON knowledge_base_crud.knowledge_drafts(status, created_at DESC);
