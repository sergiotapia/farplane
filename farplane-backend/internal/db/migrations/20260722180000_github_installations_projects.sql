-- +goose Up
-- GitHub App installations, cached repositories, and Projects bound to repos.

CREATE TABLE github_installations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    github_installation_id  BIGINT NOT NULL,
    github_account_id       BIGINT NOT NULL,
    github_account_login    TEXT NOT NULL,
    github_account_type     TEXT NOT NULL,
    repository_selection    TEXT NOT NULL,
    connected_by_user_id    UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    suspended_at            TIMESTAMPTZ(6),
    uninstalled_at          TIMESTAMPTZ(6),
    created_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT github_installations_github_installation_id_uidx UNIQUE (github_installation_id),
    CONSTRAINT github_installations_account_login_nonempty CHECK (length(trim(github_account_login)) > 0),
    CONSTRAINT github_installations_account_type_check CHECK (github_account_type IN ('User', 'Organization')),
    CONSTRAINT github_installations_repository_selection_check CHECK (repository_selection IN ('all', 'selected'))
);

CREATE INDEX github_installations_organization_id_idx
    ON github_installations (organization_id)
    WHERE uninstalled_at IS NULL;

CREATE TABLE github_repositories (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_installation_id  UUID NOT NULL REFERENCES github_installations (id) ON DELETE CASCADE,
    github_repository_id    BIGINT NOT NULL,
    full_name               TEXT NOT NULL,
    default_branch          TEXT NOT NULL DEFAULT 'main',
    private                 BOOLEAN NOT NULL DEFAULT true,
    html_url                TEXT NOT NULL DEFAULT '',
    removed_at              TIMESTAMPTZ(6),
    created_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT github_repositories_installation_repository_uidx
        UNIQUE (github_installation_id, github_repository_id),
    CONSTRAINT github_repositories_full_name_nonempty CHECK (length(trim(full_name)) > 0)
);

CREATE INDEX github_repositories_github_repository_id_idx
    ON github_repositories (github_repository_id)
    WHERE removed_at IS NULL;

CREATE TABLE projects (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id          UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    github_repository_id     BIGINT NOT NULL,
    github_installation_id   UUID NOT NULL REFERENCES github_installations (id) ON DELETE RESTRICT,
    default_branch           TEXT NOT NULL DEFAULT 'main',
    github_full_name         TEXT NOT NULL,
    github_access_status     TEXT NOT NULL DEFAULT 'active',
    created_by_user_id       UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at               TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT projects_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT projects_github_full_name_nonempty CHECK (length(trim(github_full_name)) > 0),
    CONSTRAINT projects_github_access_status_check CHECK (github_access_status IN ('active', 'revoked')),
    CONSTRAINT projects_organization_github_repository_uidx UNIQUE (organization_id, github_repository_id)
);

CREATE INDEX projects_organization_id_idx ON projects (organization_id);

-- +goose Down
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS github_repositories;
DROP TABLE IF EXISTS github_installations;
