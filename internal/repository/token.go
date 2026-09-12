package repository

import (
	"context"
	"database/sql"
	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type TokenRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewTokenRepository(db *sqlx.DB, log *zap.Logger) *TokenRepo {
	return &TokenRepo{
		db:  db,
		log: log,
	}
}

func (t *TokenRepo) CreateToken(ctx context.Context, token models.Token) error {
	query := `INSERT INTO tokens (user_id, token_id, role, expired_at) VALUES (?, ?, ?, ?)`

	_, err := t.db.ExecContext(ctx, query, token.UserID, token.TokenID, token.Role, token.ExpiresAt)
	if err != nil {
		t.log.Error("failed to execute INSERT query in CreateToken",
			zap.Error(err),
			zap.String("token_id", token.TokenID),
			zap.String("user_id", token.UserID),
		)
		return err
	}

	return nil
}

func (t *TokenRepo) Token(ctx context.Context, tokenID string) (models.Token, error) {
	query := `SELECT user_id, token_id, role, expired_at FROM tokens WHERE token_id=?`

	row := t.db.QueryRowContext(ctx, query, tokenID)

	var token models.Token
	if err := row.Scan(
		&token.UserID,
		&token.TokenID,
		&token.Role,
		&token.ExpiresAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.Token{}, err
		}
		t.log.Error("database error in Token query",
			zap.Error(err),
			zap.String("token_id", tokenID),
		)
		return models.Token{}, err
	}

	return token, nil
}

func (t *TokenRepo) DeleteToken(ctx context.Context, tokenID string) error {
	query := `DELETE FROM tokens WHERE token_id=?`

	result, err := t.db.ExecContext(ctx, query, tokenID)
	if err != nil {
		t.log.Error("failed to execute DELETE query",
			zap.Error(err),
			zap.String("token_id", tokenID),
		)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.log.Error("failed to get rows affected after DELETE",
			zap.Error(err),
			zap.String("token_id", tokenID),
		)
		return err
	}

	if rowsAffected == 0 {
		return err
	}

	return nil
}
