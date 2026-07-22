-- +goose Up
-- Opaque browser sessions (revoke on logout).
-- token stores a SHA-256 hex digest of the cookie value (never the raw secret).

CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token      TEXT NOT NULL,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ(6) NOT NULL,
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT sessions_token_nonempty CHECK (length(trim(token)) > 0)
);

CREATE UNIQUE INDEX sessions_token_uidx ON sessions (token);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
