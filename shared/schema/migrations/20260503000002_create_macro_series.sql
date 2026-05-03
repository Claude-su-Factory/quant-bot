-- +goose Up
CREATE TABLE macro_series (
    series_id   TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    value       NUMERIC(20, 8),
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source      TEXT NOT NULL DEFAULT 'fred',
    PRIMARY KEY (series_id, observed_at)
);
SELECT create_hypertable('macro_series', 'observed_at');

-- +goose Down
DROP TABLE macro_series;
