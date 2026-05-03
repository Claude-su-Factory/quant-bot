-- +goose Up
CREATE TABLE macro_series (
    series_id   TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    value       NUMERIC,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source      TEXT NOT NULL DEFAULT 'fred',
    PRIMARY KEY (series_id, observed_at)
);
SELECT create_hypertable('macro_series', 'observed_at');

CREATE INDEX macro_series_series_idx ON macro_series (series_id, observed_at DESC);

-- +goose Down
DROP TABLE macro_series;
