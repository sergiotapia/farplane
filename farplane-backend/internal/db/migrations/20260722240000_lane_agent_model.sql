-- +goose Up
-- Persist selected agent model and reasoning effort on each Lane.

ALTER TABLE lanes
    ADD COLUMN agent_model TEXT,
    ADD COLUMN reasoning_effort TEXT;

UPDATE lanes
SET agent_model = CASE agent_provider
        WHEN 'claude_code' THEN 'claude-sonnet-4-5'
        WHEN 'codex' THEN 'gpt-5.1-codex'
        WHEN 'opencode' THEN 'anthropic/claude-sonnet-4.5'
        WHEN 'oh_my_pi' THEN 'anthropic/claude-sonnet-4.5'
        ELSE 'claude-sonnet-4-5'
    END,
    reasoning_effort = 'medium'
WHERE agent_model IS NULL;

ALTER TABLE lanes
    ALTER COLUMN agent_model SET NOT NULL;

ALTER TABLE lanes
    ADD CONSTRAINT lanes_agent_model_nonempty CHECK (length(trim(agent_model)) > 0);

-- +goose Down
ALTER TABLE lanes
    DROP CONSTRAINT IF EXISTS lanes_agent_model_nonempty;

ALTER TABLE lanes
    DROP COLUMN IF EXISTS reasoning_effort,
    DROP COLUMN IF EXISTS agent_model;
