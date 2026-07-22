-- +goose Up
-- Credentials for this install's GitHub App (from manifest flow or operator paste).
-- One Farplane install has at most one GitHub App.

CREATE TABLE github_app_credentials (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_app_id               BIGINT NOT NULL,
    github_app_slug             TEXT NOT NULL,
    private_key_pem_encrypted   BYTEA NOT NULL,
    webhook_secret_encrypted    BYTEA NOT NULL,
    client_id_encrypted         BYTEA,
    client_secret_encrypted     BYTEA,
    created_by_user_id          UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT github_app_credentials_slug_nonempty CHECK (length(trim(github_app_slug)) > 0)
);

-- Singleton: one credential row per Farplane install.
CREATE UNIQUE INDEX github_app_credentials_singleton_uidx ON github_app_credentials ((true));

-- +goose Down
DROP TABLE IF EXISTS github_app_credentials;
