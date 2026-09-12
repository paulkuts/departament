package repository

import (
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type Repository struct {
	Article   *ArticleRepo
	Event     *EventRepo
	User      *UserRepo
	Key       *KeyRepo
	KeyLog    *KeyLogRepo
	Equipment *InventoryRepo
	Photo     *PhotoRepo
	Token     *TokenRepo
}

func New(db *sqlx.DB, log *zap.Logger) *Repository {
	return &Repository{
		Article:   NewArticleRepo(db, log),
		Event:     NewEventRepository(db, log),
		User:      NewUserRepo(db, log),
		Key:       NewKeyRepo(db, log),
		KeyLog:    NewKeyLogRepo(db, log),
		Equipment: NewInventoryRepo(db, log),
		Photo:     NewPhotoRepo(db, log),
		Token:     NewTokenRepository(db, log),
	}
}
