package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type KeyRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewKeyRepo(db *sqlx.DB, log *zap.Logger) *KeyRepo {
	return &KeyRepo{db: db, log: log}
}

// Create создаёт новый ключ
func (r *KeyRepo) Create(ctx context.Context, k *models.Key) error {
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO keys (key_number, room_description, status, notes)
		 VALUES (:key_number, :room_description, :status, :notes)`, k)
	if err != nil {
		return fmt.Errorf("insert key: %w", err)
	}
	id, _ := res.LastInsertId()
	k.ID = id
	return nil
}

// GetByID возвращает ключ по ID
func (r *KeyRepo) GetByID(ctx context.Context, id int64) (*models.Key, error) {
	k := &models.Key{}
	err := r.db.GetContext(ctx, k,
		`SELECT id, key_number, room_description, status, notes
		 FROM keys WHERE id = ?`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get key: %w", err)
	}
	return k, nil
}

// GetByKeyNumber возвращает ключ по номеру
func (r *KeyRepo) GetByKeyNumber(ctx context.Context, keyNumber string) (*models.Key, error) {
	k := &models.Key{}
	err := r.db.GetContext(ctx, k,
		`SELECT id, key_number, room_description, status, notes
		 FROM keys WHERE key_number = ?`, keyNumber)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get key by number: %w", err)
	}
	return k, nil
}

// ListAll возвращает все ключи
func (r *KeyRepo) ListAll(ctx context.Context) ([]models.Key, error) {
	var keys []models.Key
	err := r.db.SelectContext(ctx, &keys,
		`SELECT id, key_number, room_description, status, notes FROM keys ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	return keys, nil
}

// ListByStatus возвращает ключи по статусу
func (r *KeyRepo) ListByStatus(ctx context.Context, status models.KeyStatus) ([]models.Key, error) {
	var keys []models.Key
	err := r.db.SelectContext(ctx, &keys,
		`SELECT id, key_number, room_description, status, notes 
		 FROM keys WHERE status = ? ORDER BY id`, status)
	if err != nil {
		return nil, fmt.Errorf("list keys by status: %w", err)
	}
	return keys, nil
}

// UpdateStatus атомарно меняет статус ключа
func (r *KeyRepo) UpdateStatus(ctx context.Context, id int64, status models.KeyStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE keys SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("key not found")
	}
	return nil
}

// Update обновляет данные ключа
func (r *KeyRepo) Update(ctx context.Context, k *models.Key) error {
	_, err := r.db.NamedExecContext(ctx,
		`UPDATE keys SET 
			key_number = :key_number,
			room_description = :room_description,
			status = :status,
			notes = :notes
		 WHERE id = :id`, k)
	if err != nil {
		return fmt.Errorf("update key: %w", err)
	}
	return nil
}
