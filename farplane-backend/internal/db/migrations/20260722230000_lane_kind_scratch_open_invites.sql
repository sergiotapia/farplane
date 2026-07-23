-- +goose Up
-- Scratch Lanes (nullable project), hard-delete seats, open multi-use Lane Invites.

ALTER TABLE lanes
    ALTER COLUMN project_id DROP NOT NULL;

ALTER TABLE lanes
    ADD COLUMN lane_kind TEXT;

UPDATE lanes SET lane_kind = 'project' WHERE lane_kind IS NULL;

ALTER TABLE lanes
    ALTER COLUMN lane_kind SET NOT NULL;

ALTER TABLE lanes
    ADD CONSTRAINT lanes_lane_kind_check CHECK (lane_kind IN ('project', 'scratch'));

ALTER TABLE lanes
    ADD CONSTRAINT lanes_lane_kind_project_id_check CHECK (
        (lane_kind = 'project' AND project_id IS NOT NULL)
        OR (lane_kind = 'scratch' AND project_id IS NULL)
    );

DROP INDEX IF EXISTS lane_participants_active_lane_idx;

ALTER TABLE lane_participants
    DROP CONSTRAINT IF EXISTS lane_participants_removed_pair_check;

ALTER TABLE lane_participants
    DROP COLUMN IF EXISTS removed_at,
    DROP COLUMN IF EXISTS removed_by_user_id;

ALTER TABLE lane_invites
    DROP CONSTRAINT IF EXISTS lane_invites_target_check;

ALTER TABLE lane_invites
    DROP CONSTRAINT IF EXISTS lane_invites_accepted_pair_check;

DROP INDEX IF EXISTS lane_invites_email_lower_idx;
DROP INDEX IF EXISTS lane_invites_invited_user_id_idx;

ALTER TABLE lane_invites
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS invited_user_id,
    DROP COLUMN IF EXISTS accepted_at,
    DROP COLUMN IF EXISTS accepted_by_user_id;

CREATE UNIQUE INDEX lane_invites_one_non_revoked_per_lane_uidx
    ON lane_invites (lane_id)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS lane_invites_one_non_revoked_per_lane_uidx;

-- Open multi-use invites have no target; remove them before restoring the old check.
DELETE FROM lane_invites;

ALTER TABLE lane_invites
    ADD COLUMN email TEXT,
    ADD COLUMN invited_user_id UUID REFERENCES users (id) ON DELETE CASCADE,
    ADD COLUMN accepted_at TIMESTAMPTZ(6),
    ADD COLUMN accepted_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL;

ALTER TABLE lane_invites
    ADD CONSTRAINT lane_invites_target_check CHECK (
        email IS NOT NULL OR invited_user_id IS NOT NULL
    );

ALTER TABLE lane_invites
    ADD CONSTRAINT lane_invites_accepted_pair_check CHECK (
        (accepted_at IS NULL AND accepted_by_user_id IS NULL)
        OR (accepted_at IS NOT NULL AND accepted_by_user_id IS NOT NULL)
    );

CREATE INDEX lane_invites_email_lower_idx
    ON lane_invites (lower(email)) WHERE email IS NOT NULL;
CREATE INDEX lane_invites_invited_user_id_idx
    ON lane_invites (invited_user_id)
    WHERE invited_user_id IS NOT NULL AND accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE lane_participants
    ADD COLUMN removed_at TIMESTAMPTZ(6),
    ADD COLUMN removed_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL;

ALTER TABLE lane_participants
    ADD CONSTRAINT lane_participants_removed_pair_check CHECK (
        (removed_at IS NULL AND removed_by_user_id IS NULL)
        OR (removed_at IS NOT NULL)
    );

CREATE INDEX lane_participants_active_lane_idx
    ON lane_participants (lane_id)
    WHERE removed_at IS NULL;

ALTER TABLE lanes
    DROP CONSTRAINT IF EXISTS lanes_lane_kind_project_id_check;

ALTER TABLE lanes
    DROP CONSTRAINT IF EXISTS lanes_lane_kind_check;

ALTER TABLE lanes
    DROP COLUMN IF EXISTS lane_kind;

-- Scratch rows cannot satisfy NOT NULL project_id; delete them before restore.
DELETE FROM lanes WHERE project_id IS NULL;

ALTER TABLE lanes
    ALTER COLUMN project_id SET NOT NULL;
