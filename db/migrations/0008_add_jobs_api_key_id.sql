-- +goose Up
ALTER TABLE jobs
    ADD COLUMN api_key_id UUID;

ALTER TABLE jobs
    ADD CONSTRAINT fk_jobs_api_key
    FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS fk_jobs_api_key;
ALTER TABLE jobs DROP COLUMN IF EXISTS api_key_id;
