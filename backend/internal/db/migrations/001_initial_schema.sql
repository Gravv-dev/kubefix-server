-- 001_initial_schema.sql
-- KubeFix Labs initial schema.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users: one row per authenticated Google account.
-- Identity is the Google "sub" (subject) identifier, never an email or frontend-supplied ID.
CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    google_sub TEXT NOT NULL UNIQUE,
    email      TEXT NOT NULL,
    name       TEXT NOT NULL,
    picture    TEXT,
    role       TEXT NOT NULL DEFAULT 'user',           -- user | admin
    credits    INTEGER NOT NULL DEFAULT 10,            -- challenge-start credits (free tier allowance)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_google_sub ON users(google_sub);
CREATE INDEX idx_users_email ON users(email);

-- Challenges: definable by admins, solved by users in isolated environments.
CREATE TABLE challenges (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    category        TEXT NOT NULL,                    -- docker | kubernetes
    difficulty      TEXT NOT NULL,                    -- easy | medium | hard
    instructions    TEXT NOT NULL,                    -- user-facing challenge brief
    hints           JSONB NOT NULL DEFAULT '[]',      -- array of hint strings
    points          INTEGER NOT NULL DEFAULT 100,
    is_published    BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_challenges_category ON challenges(category);
CREATE INDEX idx_challenges_published ON challenges(is_published);

-- Attempts: a user's active/completed attempt at a challenge.
-- One credit is consumed when an environment is successfully allocated.
CREATE TABLE attempts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge_id      UUID NOT NULL REFERENCES challenges(id) ON DELETE CASCADE,
    status            TEXT NOT NULL DEFAULT 'in_progress', -- in_progress | passed | failed | expired
    environment_id    TEXT,                              -- container id / cluster ref
    environment_type  TEXT,                              -- docker | kubernetes
    credits_consumed  INTEGER NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at          TIMESTAMPTZ,
    solved_at         TIMESTAMPTZ,
    solve_time_seconds INTEGER
);

CREATE INDEX idx_attempts_user ON attempts(user_id);
CREATE INDEX idx_attempts_challenge ON attempts(challenge_id);
CREATE INDEX idx_attempts_status ON attempts(status);

-- Submissions: solution checks for an attempt.
-- A failed check does NOT consume a start credit.
CREATE TABLE submissions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id UUID NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    answer     TEXT NOT NULL,
    passed     BOOLEAN NOT NULL DEFAULT false,
    feedback   TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_submissions_attempt ON submissions(attempt_id);

-- Audit log for credit mutations (tamper-evident trail).
CREATE TABLE credit_events (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempt_id UUID REFERENCES attempts(id) ON DELETE SET NULL,
    delta     INTEGER NOT NULL,                       -- negative = consumed
    reason    TEXT NOT NULL,                          -- challenge_start | restart | admin_adjust | refund
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_credit_events_user ON credit_events(user_id);
