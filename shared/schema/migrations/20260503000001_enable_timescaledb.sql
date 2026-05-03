-- +goose Up
-- +goose NO TRANSACTION
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- +goose Down
-- +goose NO TRANSACTION
DROP EXTENSION IF EXISTS timescaledb;
