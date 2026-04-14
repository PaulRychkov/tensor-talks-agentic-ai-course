-- 001_create_schemas.sql
-- Creates PostgreSQL schemas for all TensorTalks microservices.
-- NOTE: schema user_store renamed to user_crud (§10.9).
-- Run BEFORE service-specific migrations (002_*.sql and later).

CREATE SCHEMA IF NOT EXISTS user_crud;
CREATE SCHEMA IF NOT EXISTS session_crud;
CREATE SCHEMA IF NOT EXISTS chat_crud;
CREATE SCHEMA IF NOT EXISTS results_crud;
CREATE SCHEMA IF NOT EXISTS knowledge_base_crud;
CREATE SCHEMA IF NOT EXISTS questions_crud;

-- Backward-compat alias so existing data doesn't break during migration
-- Remove after all services are updated to use user_crud schema.
CREATE SCHEMA IF NOT EXISTS user_store;

-- Grant permissions (local dev uses postgres superuser; production uses team10 role)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'team10') THEN
        GRANT ALL PRIVILEGES ON SCHEMA user_crud TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA user_store TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA session_crud TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA chat_crud TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA results_crud TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA knowledge_base_crud TO team10;
        GRANT ALL PRIVILEGES ON SCHEMA questions_crud TO team10;
        GRANT USAGE ON SCHEMA user_crud TO team10;
        GRANT USAGE ON SCHEMA user_store TO team10;
        GRANT USAGE ON SCHEMA session_crud TO team10;
        GRANT USAGE ON SCHEMA chat_crud TO team10;
        GRANT USAGE ON SCHEMA results_crud TO team10;
        GRANT USAGE ON SCHEMA knowledge_base_crud TO team10;
        GRANT USAGE ON SCHEMA questions_crud TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA user_crud GRANT ALL ON TABLES TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA session_crud GRANT ALL ON TABLES TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA chat_crud GRANT ALL ON TABLES TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA results_crud GRANT ALL ON TABLES TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA knowledge_base_crud GRANT ALL ON TABLES TO team10;
        ALTER DEFAULT PRIVILEGES IN SCHEMA questions_crud GRANT ALL ON TABLES TO team10;
    END IF;
END
$$;
