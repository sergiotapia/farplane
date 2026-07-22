-- +goose Up
-- Lane templates, org secrets, lanes, participants, invites, and messages.

CREATE TABLE lane_templates (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    description             TEXT NOT NULL DEFAULT '',
    dockerfile_text         TEXT NOT NULL,
    is_system_default       BOOLEAN NOT NULL DEFAULT false,
    forked_from_template_id UUID REFERENCES lane_templates (id) ON DELETE SET NULL,
    created_by_user_id      UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id      UUID REFERENCES users (id) ON DELETE SET NULL,
    validation_status       TEXT NOT NULL DEFAULT 'invalid',
    validated_image_reference     TEXT,
    last_validation_log     TEXT,
    validated_at            TIMESTAMPTZ(6),
    created_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT lane_templates_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT lane_templates_dockerfile_nonempty CHECK (length(trim(dockerfile_text)) > 0),
    CONSTRAINT lane_templates_validation_status_check CHECK (
        validation_status IN ('valid', 'invalid')
    ),
    CONSTRAINT lane_templates_organization_name_uidx UNIQUE (organization_id, name)
);

CREATE INDEX lane_templates_organization_id_idx ON lane_templates (organization_id);

CREATE TABLE organization_secrets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    value_encrypted    BYTEA NOT NULL,
    created_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT organization_secrets_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT organization_secrets_organization_name_uidx UNIQUE (organization_id, name)
);

CREATE INDEX organization_secrets_organization_id_idx ON organization_secrets (organization_id);

CREATE TABLE lanes (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id                  UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    owner_user_id               UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    name                        TEXT NOT NULL DEFAULT '',
    lane_template_id            UUID REFERENCES lane_templates (id) ON DELETE SET NULL,
    dockerfile_snapshot         TEXT NOT NULL,
    image_reference             TEXT,
    runtime_kind                TEXT NOT NULL,
    runtime_id                  TEXT,
    agent_provider              TEXT NOT NULL,
    agent_provider_session_id   TEXT,
    status                      TEXT NOT NULL DEFAULT 'creating',
    created_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT lanes_runtime_kind_check CHECK (runtime_kind IN ('docker', 'sprites')),
    CONSTRAINT lanes_dockerfile_snapshot_nonempty CHECK (
        length(trim(dockerfile_snapshot)) > 0
    ),
    CONSTRAINT lanes_agent_provider_check CHECK (
        agent_provider IN ('claude_code', 'codex', 'opencode', 'oh_my_pi')
    ),
    CONSTRAINT lanes_status_check CHECK (
        status IN ('creating', 'running', 'sleeping', 'error', 'destroyed')
    )
);

CREATE INDEX lanes_project_id_idx ON lanes (project_id);
CREATE INDEX lanes_organization_id_idx ON lanes (organization_id);
CREATE INDEX lanes_owner_user_id_idx ON lanes (owner_user_id);

CREATE TABLE lane_participants (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lane_id              UUID NOT NULL REFERENCES lanes (id) ON DELETE CASCADE,
    user_id              UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role                 TEXT NOT NULL DEFAULT 'participant',
    joined_at            TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    removed_at           TIMESTAMPTZ(6),
    removed_by_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT lane_participants_role_check CHECK (role IN ('owner', 'participant')),
    CONSTRAINT lane_participants_lane_user_uidx UNIQUE (lane_id, user_id),
    CONSTRAINT lane_participants_removed_pair_check CHECK (
        (removed_at IS NULL AND removed_by_user_id IS NULL)
        OR (removed_at IS NOT NULL)
    )
);

CREATE INDEX lane_participants_user_id_idx ON lane_participants (user_id);
CREATE INDEX lane_participants_active_lane_idx
    ON lane_participants (lane_id)
    WHERE removed_at IS NULL;

CREATE TABLE lane_invites (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lane_id              UUID NOT NULL REFERENCES lanes (id) ON DELETE CASCADE,
    token                TEXT NOT NULL,
    email                TEXT,
    invited_user_id      UUID REFERENCES users (id) ON DELETE CASCADE,
    invited_by_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    expires_at           TIMESTAMPTZ(6),
    accepted_at          TIMESTAMPTZ(6),
    accepted_by_user_id  UUID REFERENCES users (id) ON DELETE SET NULL,
    revoked_at           TIMESTAMPTZ(6),
    created_at           TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT lane_invites_token_nonempty CHECK (length(trim(token)) > 0),
    CONSTRAINT lane_invites_token_uidx UNIQUE (token),
    CONSTRAINT lane_invites_target_check CHECK (
        email IS NOT NULL OR invited_user_id IS NOT NULL
    ),
    CONSTRAINT lane_invites_accepted_pair_check CHECK (
        (accepted_at IS NULL AND accepted_by_user_id IS NULL)
        OR (accepted_at IS NOT NULL AND accepted_by_user_id IS NOT NULL)
    )
);

CREATE INDEX lane_invites_lane_id_idx ON lane_invites (lane_id);
CREATE INDEX lane_invites_email_lower_idx ON lane_invites (lower(email)) WHERE email IS NOT NULL;
CREATE INDEX lane_invites_invited_user_id_idx ON lane_invites (invited_user_id)
    WHERE invited_user_id IS NOT NULL AND accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE lane_messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lane_id          UUID NOT NULL REFERENCES lanes (id) ON DELETE CASCADE,
    sequence_number  BIGINT NOT NULL,
    event_type       TEXT NOT NULL,
    role             TEXT,
    author_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    body             TEXT,
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT lane_messages_lane_sequence_uidx UNIQUE (lane_id, sequence_number),
    CONSTRAINT lane_messages_role_check CHECK (
        role IS NULL OR role IN ('user', 'assistant', 'system')
    )
);

CREATE INDEX lane_messages_lane_created_at_idx ON lane_messages (lane_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS lane_messages;
DROP TABLE IF EXISTS lane_invites;
DROP TABLE IF EXISTS lane_participants;
DROP TABLE IF EXISTS lanes;
DROP TABLE IF EXISTS organization_secrets;
DROP TABLE IF EXISTS lane_templates;
