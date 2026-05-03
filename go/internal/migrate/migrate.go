// Package migrate는 goose 기반 SQL 마이그레이션을 관리한다.
// SQL 파일은 build 시 shared/schema/migrations/에서 동기화되어 임베드된다 (R9 단일 진실).
package migrate

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ErrMigrationsPending는 적용 안 된 마이그레이션이 있을 때 반환된다.
var ErrMigrationsPending = errors.New("미적용 마이그레이션 발견")

// Up은 모든 미적용 마이그레이션을 적용한다.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("goose Up: %w", err)
	}
	return nil
}

// Status는 적용/미적용 마이그레이션 목록을 stdout에 출력한다.
func Status(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	if err := goose.StatusContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("goose Status: %w", err)
	}
	return nil
}

// AssertUpToDate는 미적용 마이그레이션 있으면 ErrMigrationsPending을 반환한다.
// Bootstrap의 fail-fast 검증에 사용 (R12).
func AssertUpToDate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("DB 버전 조회 실패: %w", err)
	}
	migrations, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("마이그레이션 목록 조회 실패: %w", err)
	}
	last := migrations[len(migrations)-1].Version
	if current < last {
		return fmt.Errorf("%w: DB %d, 최신 %d", ErrMigrationsPending, current, last)
	}
	return nil
}
