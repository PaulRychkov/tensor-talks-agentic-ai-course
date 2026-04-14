-- 002_user_crud.sql
-- user-crud-service (бывший user-store-service) tables.
-- Includes login word dictionaries and recovery key support (§10.10).

-- Main users table
CREATE TABLE IF NOT EXISTS user_crud.users (
    id                      BIGSERIAL PRIMARY KEY,
    external_id             UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    login                   VARCHAR(64) NOT NULL UNIQUE,
    password_hash           TEXT NOT NULL,
    recovery_key_hash       VARCHAR(256),              -- §10.10: bcrypt hash of recovery key
    recovery_attempts       INT NOT NULL DEFAULT 0,
    recovery_blocked_until  TIMESTAMP WITH TIME ZONE,
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_login ON user_crud.users(login);

-- Login adjectives dictionary (§10.10)
CREATE TABLE IF NOT EXISTS user_crud.login_adjectives (
    id          SERIAL PRIMARY KEY,
    word        VARCHAR(30) NOT NULL UNIQUE,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_login_adjectives_active
    ON user_crud.login_adjectives(is_active) WHERE is_active = TRUE;

-- Login nouns dictionary (§10.10)
CREATE TABLE IF NOT EXISTS user_crud.login_nouns (
    id          SERIAL PRIMARY KEY,
    word        VARCHAR(30) NOT NULL UNIQUE,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_login_nouns_active
    ON user_crud.login_nouns(is_active) WHERE is_active = TRUE;

-- User roles table (§10.1)
CREATE TABLE IF NOT EXISTS user_crud.user_roles (
    user_id     BIGINT NOT NULL REFERENCES user_crud.users(id),
    role        VARCHAR(20) NOT NULL CHECK (role IN ('user', 'content_editor', 'admin')),
    granted_by  BIGINT REFERENCES user_crud.users(id),
    granted_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, role)
);
