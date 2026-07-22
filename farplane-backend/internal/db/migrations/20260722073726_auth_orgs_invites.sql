-- +goose Up
-- Auth, organizations, membership, and invite links.

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL,
    password_hash   TEXT, -- null when the user only signs in with Google
    display_name    TEXT NOT NULL DEFAULT '',
    avatar_url      TEXT,
    created_at      TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT users_email_nonempty CHECK (length(trim(email)) > 0)
);

CREATE UNIQUE INDEX users_email_lower_uidx ON users (lower(email));

-- External sign-in providers (Google today). Email/password lives on users.
CREATE TABLE user_identities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    created_at       TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT user_identities_provider_nonempty CHECK (length(trim(provider)) > 0),
    CONSTRAINT user_identities_subject_nonempty CHECK (length(trim(provider_subject)) > 0),
    CONSTRAINT user_identities_provider_subject_uidx UNIQUE (provider, provider_subject),
    CONSTRAINT user_identities_user_provider_uidx UNIQUE (user_id, provider)
);

CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT organizations_name_nonempty CHECK (length(trim(name)) > 0)
);

CREATE TABLE organization_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'member',
    created_at      TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT organization_members_role_check CHECK (role IN ('owner', 'admin', 'member')),
    CONSTRAINT organization_members_organization_user_uidx UNIQUE (organization_id, user_id)
);

CREATE INDEX organization_members_user_id_idx ON organization_members (user_id);

-- Shareable invite URLs (copy/paste). Token is the secret in the invite link.
CREATE TABLE organization_invites (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    token               TEXT NOT NULL,
    email               TEXT, -- optional target; null = open link for any signup
    role                TEXT NOT NULL DEFAULT 'member',
    invited_by_user_id  UUID REFERENCES users (id) ON DELETE SET NULL,
    expires_at          TIMESTAMPTZ(6),
    accepted_at         TIMESTAMPTZ(6),
    accepted_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    revoked_at          TIMESTAMPTZ(6),
    created_at          TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
    CONSTRAINT organization_invites_token_nonempty CHECK (length(trim(token)) > 0),
    CONSTRAINT organization_invites_token_uidx UNIQUE (token),
    CONSTRAINT organization_invites_role_check CHECK (role IN ('owner', 'admin', 'member')),
    CONSTRAINT organization_invites_accepted_pair_check CHECK (
        (accepted_at IS NULL AND accepted_by_user_id IS NULL)
        OR (accepted_at IS NOT NULL AND accepted_by_user_id IS NOT NULL)
    )
);

CREATE INDEX organization_invites_organization_id_idx ON organization_invites (organization_id);
CREATE INDEX organization_invites_email_lower_idx ON organization_invites (lower(email)) WHERE email IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS organization_invites;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS user_identities;
DROP TABLE IF EXISTS users;
