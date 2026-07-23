-- +goose Up
-- +goose StatementBegin
ALTER TABLE lanes
    ADD COLUMN model_source TEXT;

UPDATE lanes
SET model_source = CASE agent_provider
    WHEN 'claude_code' THEN 'anthropic'
    WHEN 'codex' THEN 'openai'
    ELSE 'openrouter'
END
WHERE model_source IS NULL;

ALTER TABLE lanes
    ALTER COLUMN model_source SET NOT NULL;

ALTER TABLE lanes
    ADD CONSTRAINT lanes_model_source_nonempty CHECK (length(trim(model_source)) > 0);

ALTER TABLE lanes
    ADD CONSTRAINT lanes_model_source_known CHECK (
        model_source IN ('anthropic', 'openai', 'openrouter')
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE lanes
    DROP CONSTRAINT IF EXISTS lanes_model_source_known;

ALTER TABLE lanes
    DROP CONSTRAINT IF EXISTS lanes_model_source_nonempty;

ALTER TABLE lanes
    DROP COLUMN IF EXISTS model_source;
-- +goose StatementEnd
