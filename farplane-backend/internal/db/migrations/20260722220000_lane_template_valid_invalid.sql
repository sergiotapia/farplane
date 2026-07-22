-- +goose Up
ALTER TABLE lane_templates
    DROP CONSTRAINT IF EXISTS lane_templates_validation_status_check;

UPDATE lane_templates
SET validation_status = 'invalid'
WHERE validation_status IS DISTINCT FROM 'valid';

ALTER TABLE lane_templates
    ALTER COLUMN validation_status SET DEFAULT 'invalid';

ALTER TABLE lane_templates
    ADD CONSTRAINT lane_templates_validation_status_check CHECK (
        validation_status IN ('valid', 'invalid')
    );

-- +goose Down
ALTER TABLE lane_templates
    DROP CONSTRAINT IF EXISTS lane_templates_validation_status_check;

ALTER TABLE lane_templates
    ALTER COLUMN validation_status SET DEFAULT 'draft';

UPDATE lane_templates
SET validation_status = 'draft'
WHERE validation_status = 'invalid';

ALTER TABLE lane_templates
    ADD CONSTRAINT lane_templates_validation_status_check CHECK (
        validation_status IN (
            'draft', 'lint_failed', 'validating', 'valid', 'build_failed', 'stale'
        )
    );
