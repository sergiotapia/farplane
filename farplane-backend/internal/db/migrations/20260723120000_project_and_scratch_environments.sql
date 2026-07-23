-- +goose Up
-- Replace org lane templates with Scratch Environment (1/org) and
-- Project Environment (0..1/project).

CREATE TABLE scratch_environments (
    organization_id             UUID PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    dockerfile_text             TEXT NOT NULL,
    validation_status           TEXT NOT NULL DEFAULT 'invalid',
    validated_image_reference   TEXT,
    last_validation_log         TEXT,
    validated_at                TIMESTAMPTZ(6),
    updated_by_user_id          UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT scratch_environments_dockerfile_nonempty CHECK (
        length(trim(dockerfile_text)) > 0
    ),
    CONSTRAINT scratch_environments_validation_status_check CHECK (
        validation_status IN ('valid', 'invalid')
    )
);

CREATE TABLE project_environments (
    project_id                  UUID PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    organization_id             UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    dockerfile_text             TEXT NOT NULL,
    validation_status           TEXT NOT NULL DEFAULT 'invalid',
    validated_image_reference   TEXT,
    last_validation_log         TEXT,
    validated_at                TIMESTAMPTZ(6),
    generation_status           TEXT NOT NULL DEFAULT 'idle',
    generation_log              TEXT,
    updated_by_user_id          UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT project_environments_dockerfile_nonempty CHECK (
        length(trim(dockerfile_text)) > 0
    ),
    CONSTRAINT project_environments_validation_status_check CHECK (
        validation_status IN ('valid', 'invalid')
    ),
    CONSTRAINT project_environments_generation_status_check CHECK (
        generation_status IN ('idle', 'generating', 'failed')
    )
);

CREATE INDEX project_environments_organization_id_idx
    ON project_environments (organization_id);

-- Seed Scratch Environment from each org's former system default template when present.
INSERT INTO scratch_environments (
    organization_id,
    dockerfile_text,
    validation_status,
    validated_image_reference,
    last_validation_log,
    validated_at,
    updated_by_user_id,
    created_at,
    updated_at
)
SELECT
    lt.organization_id,
    lt.dockerfile_text,
    lt.validation_status,
    lt.validated_image_reference,
    lt.last_validation_log,
    lt.validated_at,
    lt.updated_by_user_id,
    lt.created_at,
    lt.updated_at
FROM lane_templates lt
WHERE lt.is_system_default = true
ON CONFLICT (organization_id) DO NOTHING;

ALTER TABLE lanes DROP COLUMN IF EXISTS lane_template_id;

DROP TABLE IF EXISTS lane_templates;

-- +goose Down
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

ALTER TABLE lanes
    ADD COLUMN lane_template_id UUID REFERENCES lane_templates (id) ON DELETE SET NULL;

DROP TABLE IF EXISTS project_environments;
DROP TABLE IF EXISTS scratch_environments;
