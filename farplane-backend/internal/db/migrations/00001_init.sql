-- +goose Up
-- Enable UUID helpers used by later tables.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- +goose Down
-- Keep pgcrypto installed; later tables may depend on it.
SELECT 1;
