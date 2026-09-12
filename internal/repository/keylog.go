package repository

import (
	"context"
	"database/sql"
	"fmt"
	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type KeyLogRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewKeyLogRepo(db *sqlx.DB, log *zap.Logger) *KeyLogRepo {
	return &KeyLogRepo{db: db, log: log}
}

// Append записывает событие в журнал
func (r *KeyLogRepo) Append(ctx context.Context, l *models.KeyLog) error {
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO key_logs (key_id, user_id, action_type, comment)
		 VALUES (:key_id, :user_id, :action_type, :comment)`, l)
	if err != nil {
		return fmt.Errorf("insert log: %w", err)
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

// HistoryForKey возвращает историю операций по ключу
func (r *KeyLogRepo) HistoryForKey(ctx context.Context, keyID int64) ([]models.KeyLog, error) {
	var logs []models.KeyLog
	err := r.db.SelectContext(ctx, &logs,
		`SELECT id, key_id, user_id, action_type, timestamp, comment
		 FROM key_logs WHERE key_id = ? ORDER BY timestamp DESC`, keyID)
	if err != nil {
		return nil, fmt.Errorf("history for key: %w", err)
	}
	return logs, nil
}

// HistoryForUser возвращает историю операций пользователя
func (r *KeyLogRepo) HistoryForUser(ctx context.Context, userID string) ([]models.KeyLog, error) {
	var logs []models.KeyLog
	err := r.db.SelectContext(ctx, &logs,
		`SELECT id, key_id, user_id, action_type, timestamp, comment
		 FROM key_logs WHERE user_id = ? ORDER BY timestamp DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("history for user: %w", err)
	}
	return logs, nil
}

// GetCurrentHolder возвращает текущего держателя ключа (если ключ выдан)
func (r *KeyLogRepo) GetCurrentHolder(ctx context.Context, keyID int64) (*models.KeyLog, error) {
	l := &models.KeyLog{}
	err := r.db.GetContext(ctx, l,
		`SELECT id, key_id, user_id, action_type, timestamp, comment 
		 FROM key_logs 
		 WHERE key_id = ? AND action_type = ? 
		 ORDER BY timestamp DESC LIMIT 1`, keyID, models.ActionIssue)
	if err == sql.ErrNoRows {
		return nil, nil // ключ никто не держит
	}
	if err != nil {
		return nil, fmt.Errorf("get current holder: %w", err)
	}
	return l, nil
}
