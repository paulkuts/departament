package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// KeyRepo — интерфейс репозитория ключей
type KeyRepo interface {
	Create(ctx context.Context, k *models.Key) error
	GetByID(ctx context.Context, id int64) (*models.Key, error)
	GetByKeyNumber(ctx context.Context, keyNumber string) (*models.Key, error)
	ListAll(ctx context.Context) ([]models.Key, error)
	ListByStatus(ctx context.Context, status models.KeyStatus) ([]models.Key, error)
	UpdateStatus(ctx context.Context, id int64, status models.KeyStatus) error
	Update(ctx context.Context, k *models.Key) error
}

// KeyLogRepo — интерфейс репозитория журнала
type KeyLogRepo interface {
	Append(ctx context.Context, l *models.KeyLog) error
	HistoryForKey(ctx context.Context, keyID int64) ([]models.KeyLog, error)
	HistoryForUser(ctx context.Context, userID string) ([]models.KeyLog, error)
	GetCurrentHolder(ctx context.Context, keyID int64) (*models.KeyLog, error)
}

type KeyService struct {
	keyRepo KeyRepo
	logRepo KeyLogRepo
	db      *sqlx.DB // нужен для транзакций
	log     *zap.Logger
}

func NewKeyService(
	keyRepo KeyRepo,
	logRepo KeyLogRepo,
	db *sqlx.DB,
	log *zap.Logger,
) *KeyService {
	return &KeyService{
		keyRepo: keyRepo,
		logRepo: logRepo,
		db:      db,
		log:     log,
	}
}

// Create создаёт новый ключ
func (s *KeyService) Create(ctx context.Context, k *models.Key) error {
	if k.KeyNumber == "" {
		return errors.New("key number is required")
	}
	if k.Status == "" {
		k.Status = models.KeyStatusAvailable
	}

	if err := s.keyRepo.Create(ctx, k); err != nil {
		s.log.Error("failed to create key",
			zap.String("key_number", k.KeyNumber),
			zap.Error(err),
		)
		return fmt.Errorf("create key: %w", err)
	}

	s.log.Info("key created",
		zap.Int64("key_id", k.ID),
		zap.String("key_number", k.KeyNumber),
	)
	return nil
}

// GetByID возвращает ключ по ID
func (s *KeyService) GetByID(ctx context.Context, id int64) (*models.Key, error) {
	k, err := s.keyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get key by id: %w", err)
	}
	if k == nil {
		return nil, fmt.Errorf("key %d not found", id)
	}
	return k, nil
}

// GetByKeyNumber возвращает ключ по номеру
func (s *KeyService) GetByKeyNumber(ctx context.Context, keyNumber string) (*models.Key, error) {
	k, err := s.keyRepo.GetByKeyNumber(ctx, keyNumber)
	if err != nil {
		return nil, fmt.Errorf("get key by number: %w", err)
	}
	if k == nil {
		return nil, fmt.Errorf("key %q not found", keyNumber)
	}
	return k, nil
}

// ListAll возвращает все ключи
func (s *KeyService) ListAll(ctx context.Context) ([]models.Key, error) {
	keys, err := s.keyRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all keys: %w", err)
	}
	return keys, nil
}

// ListByStatus возвращает ключи по статусу
func (s *KeyService) ListByStatus(ctx context.Context, status models.KeyStatus) ([]models.Key, error) {
	keys, err := s.keyRepo.ListByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list keys by status: %w", err)
	}
	return keys, nil
}

// Issue выдаёт ключ пользователю (атомарная операция)
func (s *KeyService) Issue(ctx context.Context, keyID int64, userID string, comment string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Проверяем статус ключа
	var status models.KeyStatus
	err = tx.QueryRowxContext(ctx,
		`SELECT status FROM keys WHERE id = ?`, keyID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("key %d not found", keyID)
	}
	if err != nil {
		return fmt.Errorf("check key status: %w", err)
	}
	if status != models.KeyStatusAvailable {
		return fmt.Errorf("key %d is not available (current status: %s)", keyID, status)
	}

	// Обновляем статус ключа
	if _, err := tx.ExecContext(ctx,
		`UPDATE keys SET status = ? WHERE id = ?`, models.KeyStatusIssued, keyID); err != nil {
		return fmt.Errorf("update key status: %w", err)
	}

	// Записываем в журнал
	log := &models.KeyLog{
		KeyID:      keyID,
		UserID:     userID,
		ActionType: models.ActionIssue,
		Timestamp:  time.Now(),
		Comment:    &comment,
	}
	if _, err := tx.NamedExecContext(ctx,
		`INSERT INTO key_logs (key_id, user_id, action_type, comment) 
		 VALUES (:key_id, :user_id, :action_type, :comment)`, log); err != nil {
		return fmt.Errorf("insert issue log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	s.log.Info("key issued",
		zap.Int64("key_id", keyID),
		zap.String("user_id", userID),
	)
	return nil
}

// Return возвращает ключ (атомарная операция)
func (s *KeyService) Return(ctx context.Context, keyID int64, comment string) error {
	return s.ReturnForUser(ctx, keyID, "", comment)
}

func (s *KeyService) ReturnForUser(ctx context.Context, keyID int64, actorID, comment string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Проверяем статус ключа
	var status models.KeyStatus
	err = tx.QueryRowxContext(ctx,
		`SELECT status FROM keys WHERE id = ?`, keyID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("key %d not found", keyID)
	}
	if err != nil {
		return fmt.Errorf("check key status: %w", err)
	}
	if status != models.KeyStatusIssued {
		return fmt.Errorf("key %d is not issued (current status: %s)", keyID, status)
	}

	// Находим последнего держателя
	var userID string
	err = tx.QueryRowxContext(ctx,
		`SELECT user_id FROM key_logs 
		 WHERE key_id = ? AND action_type = ? 
		 ORDER BY timestamp DESC, id DESC LIMIT 1`,
		keyID, models.ActionIssue,
	).Scan(&userID)
	if err != nil {
		return fmt.Errorf("find last holder: %w", err)
	}

	if actorID != "" && actorID != userID {
		return fmt.Errorf("not holder")
	}
	// Обновляем статус ключа
	if _, err := tx.ExecContext(ctx,
		`UPDATE keys SET status = ? WHERE id = ?`, models.KeyStatusAvailable, keyID); err != nil {
		return fmt.Errorf("update key status: %w", err)
	}

	// Записываем возврат в журнал
	log := &models.KeyLog{
		KeyID:      keyID,
		UserID:     userID,
		ActionType: models.ActionReturn,
		Timestamp:  time.Now(),
		Comment:    &comment,
	}
	if _, err := tx.NamedExecContext(ctx,
		`INSERT INTO key_logs (key_id, user_id, action_type, comment) 
		 VALUES (:key_id, :user_id, :action_type, :comment)`, log); err != nil {
		return fmt.Errorf("insert return log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	s.log.Info("key returned",
		zap.Int64("key_id", keyID),
		zap.String("user_id", userID),
	)
	return nil
}

// Update обновляет данные ключа
func (s *KeyService) Update(ctx context.Context, k *models.Key) error {
	if k.ID == 0 {
		return errors.New("key id is required for update")
	}
	if k.KeyNumber == "" {
		return errors.New("key number is required")
	}

	if err := s.keyRepo.Update(ctx, k); err != nil {
		s.log.Error("failed to update key",
			zap.Int64("key_id", k.ID),
			zap.Error(err),
		)
		return fmt.Errorf("update key: %w", err)
	}

	s.log.Info("key updated", zap.Int64("key_id", k.ID))
	return nil
}

// MarkLost помечает ключ как утерянный. actorID — администратор, выполнивший операцию
// (журнал ссылается на users, пустой id нарушает внешний ключ).
func (s *KeyService) MarkLost(ctx context.Context, keyID int64, actorID, comment string) error {
	k, err := s.GetByID(ctx, keyID)
	if err != nil {
		return err
	}

	if k.Status == models.KeyStatusLost {
		return fmt.Errorf("key %d is already marked as lost", keyID)
	}

	if err := s.keyRepo.UpdateStatus(ctx, keyID, models.KeyStatusLost); err != nil {
		return fmt.Errorf("mark key as lost: %w", err)
	}

	// Записываем в журнал
	log := &models.KeyLog{
		KeyID:      keyID,
		UserID:     actorID, // автор операции: администратор
		ActionType: "lost",
		Timestamp:  time.Now(),
		Comment:    &comment,
	}
	if err := s.logRepo.Append(ctx, log); err != nil {
		s.log.Warn("failed to append lost log", zap.Error(err))
	}

	s.log.Info("key marked as lost", zap.Int64("key_id", keyID))
	return nil
}

// RestoreLost снимает отметку утери: ключ возвращается в реестр как доступный.
func (s *KeyService) RestoreLost(ctx context.Context, keyID int64, actorID, comment string) error {
	k, err := s.GetByID(ctx, keyID)
	if err != nil {
		return err
	}

	if k.Status != models.KeyStatusLost {
		return fmt.Errorf("key %d is not marked as lost", keyID)
	}

	if err := s.keyRepo.UpdateStatus(ctx, keyID, models.KeyStatusAvailable); err != nil {
		return fmt.Errorf("restore key after loss: %w", err)
	}

	// Записываем в журнал
	log := &models.KeyLog{
		KeyID:      keyID,
		UserID:     actorID, // автор операции: администратор
		ActionType: "restore",
		Timestamp:  time.Now(),
		Comment:    &comment,
	}
	if err := s.logRepo.Append(ctx, log); err != nil {
		s.log.Warn("failed to append restore log", zap.Error(err))
	}

	s.log.Info("key loss cancelled", zap.Int64("key_id", keyID))
	return nil
}

// HistoryForKey возвращает историю операций по ключу
func (s *KeyService) HistoryForKey(ctx context.Context, keyID int64) ([]models.KeyLog, error) {
	logs, err := s.logRepo.HistoryForKey(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("history for key: %w", err)
	}
	return logs, nil
}

// HistoryForUser возвращает историю операций пользователя
func (s *KeyService) HistoryForUser(ctx context.Context, userID string) ([]models.KeyLog, error) {
	logs, err := s.logRepo.HistoryForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("history for user: %w", err)
	}
	return logs, nil
}

// GetCurrentHolder возвращает текущего держателя ключа
func (s *KeyService) GetCurrentHolder(ctx context.Context, keyID int64) (*models.KeyLog, error) {
	holder, err := s.logRepo.GetCurrentHolder(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get current holder: %w", err)
	}
	return holder, nil
}

// Preserve audit history: only unused, non-issued keys can be physically removed.
func (s *KeyService) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM keys WHERE id = ? AND status != 'issued' AND NOT EXISTS (SELECT 1 FROM key_logs WHERE key_id = keys.id)`, id)
	if err != nil {
		return fmt.Errorf("delete key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err := s.GetByID(ctx, id)
		if err != nil {
			return err
		}
		return fmt.Errorf("cannot delete key with loans or history")
	}
	return nil
}
