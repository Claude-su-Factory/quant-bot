-- +goose Up
CREATE TABLE runs (
    id              BIGSERIAL PRIMARY KEY,
    job_name        TEXT NOT NULL,
    instance        TEXT NOT NULL DEFAULT 'paper',
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    status          TEXT NOT NULL,
    rows_processed  INTEGER NOT NULL DEFAULT 0,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT
);
CREATE INDEX runs_recent_idx ON runs (started_at DESC);
CREATE INDEX runs_job_status_idx ON runs (job_name, status, started_at DESC);

-- +goose Down
DROP TABLE runs;
