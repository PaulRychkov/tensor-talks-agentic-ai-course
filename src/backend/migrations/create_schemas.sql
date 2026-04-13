-- Migration script to create PostgreSQL schemas for team10_db
-- This script should be run once before deploying services to Kubernetes

-- Create schemas for each service
CREATE SCHEMA IF NOT EXISTS user_store;
CREATE SCHEMA IF NOT EXISTS session_crud;
CREATE SCHEMA IF NOT EXISTS chat_crud;
CREATE SCHEMA IF NOT EXISTS results_crud;
CREATE SCHEMA IF NOT EXISTS knowledge_base_crud;
CREATE SCHEMA IF NOT EXISTS questions_crud;

-- Grant necessary permissions to team10 user
GRANT ALL PRIVILEGES ON SCHEMA user_store TO team10;
GRANT ALL PRIVILEGES ON SCHEMA session_crud TO team10;
GRANT ALL PRIVILEGES ON SCHEMA chat_crud TO team10;
GRANT ALL PRIVILEGES ON SCHEMA results_crud TO team10;
GRANT ALL PRIVILEGES ON SCHEMA knowledge_base_crud TO team10;
GRANT ALL PRIVILEGES ON SCHEMA questions_crud TO team10;

-- Grant usage on all schemas
GRANT USAGE ON SCHEMA user_store TO team10;
GRANT USAGE ON SCHEMA session_crud TO team10;
GRANT USAGE ON SCHEMA chat_crud TO team10;
GRANT USAGE ON SCHEMA results_crud TO team10;
GRANT USAGE ON SCHEMA knowledge_base_crud TO team10;
GRANT USAGE ON SCHEMA questions_crud TO team10;

-- Set default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA user_store GRANT ALL ON TABLES TO team10;
ALTER DEFAULT PRIVILEGES IN SCHEMA session_crud GRANT ALL ON TABLES TO team10;
ALTER DEFAULT PRIVILEGES IN SCHEMA chat_crud GRANT ALL ON TABLES TO team10;
ALTER DEFAULT PRIVILEGES IN SCHEMA results_crud GRANT ALL ON TABLES TO team10;
ALTER DEFAULT PRIVILEGES IN SCHEMA knowledge_base_crud GRANT ALL ON TABLES TO team10;
ALTER DEFAULT PRIVILEGES IN SCHEMA questions_crud GRANT ALL ON TABLES TO team10;

