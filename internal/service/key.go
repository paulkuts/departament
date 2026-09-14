package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
		UserID:     optional(userID),
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

	// Находим последнего держателя. У гостя user_id пуст — тогда известен
	// только его телефон (в журнал он попадает вместе с ФИО).
	var holder *string
	err = tx.QueryRowxContext(ctx,
		`SELECT user_id FROM key_logs 
		 WHERE key_id = ? AND action_type = ? 
		 ORDER BY timestamp DESC, id DESC LIMIT 1`,
		keyID, models.ActionIssue,
	).Scan(&holder)
	if err != nil {
		return fmt.Errorf("find last holder: %w", err)
	}

	if actorID != "" && (holder == nil || *holder != actorID) {
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
		UserID:     holder, // возврат пишем на держателя, даже если он гость
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
		zap.Any("user_id", holder),
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
		UserID:     optional(actorID), // автор операции: администратор
		ActionType: models.ActionLost,
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
		UserID:     optional(actorID), // автор операции: администратор
		ActionType: models.ActionRestore,
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

// ScanActor — кто физически отсканировал QR на ключе.
type ScanActor struct {
	UserID string // сотрудник, если вошёл в систему
	Name   string // гость: ФИО
	Phone  string // гость: телефон
	Token  string // гость: метка браузера (cookie) — по ней узнаём его при повторном скане

	// Intent — осознанное действие с экрана: взять ключ или сдать его. Пустое
	// значение — обычный скан QR: сервер сам решает по состоянию ключа.
	Intent string
}

const (
	IntentTake   = "take"
	IntentReturn = "return"
)

// ScanOutcome — что произошло с ключом после сканирования.
type ScanOutcome struct {
	Action      models.ActionType // issue — ключ у вас, return — ключ сдан
	KeyNumber   string
	Room        string
	HolderName  string // ФИО гостя-держателя (для сотрудника имя подставляет обработчик)
	Transferred bool   // ключ перешёл от предыдущего держателя
}

var (
	// ErrKeyLost — ключ помечен утерянным: скан ничего не меняет.
	ErrKeyLost = errors.New("ключ числится утерянным")
	// ErrGuestDataNeeded — гость без метки браузера обязан назвать себя.
	ErrGuestDataNeeded = errors.New("нужны имя и телефон")
	// ErrScanConflict — состояние ключа изменилось между чтением и записью.
	ErrScanConflict = errors.New("состояние ключа изменилось, повторите скан")
	// ErrNotHolder — кнопка «сдать», но ключ уже не у этого человека.
	ErrNotHolder = errors.New("ключ сейчас не у вас")
	// ErrAlreadyHolder — кнопка «взять», но ключ уже записан за этим человеком.
	ErrAlreadyHolder = errors.New("ключ уже у вас")
)

// Scan обрабатывает сканирование QR на ключе: взять, сдать или принять ключ от
// предыдущего держателя. Статус ключа и журнал меняются в одной транзакции,
// поэтому ключ не может остаться выданным без записи о том, кто его взял.
func (s *KeyService) Scan(ctx context.Context, keyID int64, actor ScanActor) (*ScanOutcome, error) {
	actor.Name = strings.TrimSpace(actor.Name)
	actor.Phone = strings.TrimSpace(actor.Phone)

	// Гость обязан назваться, когда ключ переходит к нему: иначе в журнале
	// останется ключ, выданный неизвестно кому. Сдача по метке браузера или
	// телефону имени не требует — человека уже видно по текущей записи.
	guest := actor.UserID == ""
	guestNamed := actor.Name != "" && actor.Phone != ""
	if guest && actor.Token == "" && !guestNamed {
		return nil, ErrGuestDataNeeded
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var key struct {
		KeyNumber string           `db:"key_number"`
		Room      string           `db:"room_description"`
		Status    models.KeyStatus `db:"status"`
	}
	if err := tx.GetContext(ctx, &key,
		`SELECT key_number, room_description, status FROM keys WHERE id = ?`, keyID); err != nil {
		return nil, fmt.Errorf("key %d not found", keyID)
	}
	if key.Status == models.KeyStatusLost {
		return nil, ErrKeyLost
	}

	// Держатель — последняя выдача по ключу.
	holder := &models.KeyLog{}
	holderKnown := tx.GetContext(ctx, holder,
		`SELECT id, user_id, action_type, guest_name, guest_phone, guest_token
		 FROM key_logs WHERE key_id = ? AND action_type = ?
		 ORDER BY id DESC LIMIT 1`, keyID, models.ActionIssue) == nil

	out := &ScanOutcome{KeyNumber: key.KeyNumber, Room: key.Room}

	// Ключ свободен — выдаём тому, кто сканировал.
	if key.Status == models.KeyStatusAvailable {
		if actor.Intent == IntentReturn {
			return nil, ErrNotHolder
		}
		if guest && !guestNamed {
			return nil, ErrGuestDataNeeded
		}
		if err := s.setKeyStatus(ctx, tx, keyID, models.KeyStatusAvailable, models.KeyStatusIssued); err != nil {
			return nil, err
		}
		if err := appendScan(ctx, tx, keyID, actor, models.ActionIssue, "скан QR: выдача"); err != nil {
			return nil, err
		}
		out.Action = models.ActionIssue
		out.HolderName = actor.Name
		return out, tx.Commit()
	}

	// Ключ выдан. Сканировал тот же человек — значит сдал.
	if holderKnown && actor.matches(holder) {
		if actor.Intent == IntentTake {
			return nil, ErrAlreadyHolder
		}
		back := actorFromLog(holder)
		if err := s.setKeyStatus(ctx, tx, keyID, models.KeyStatusIssued, models.KeyStatusAvailable); err != nil {
			return nil, err
		}
		if err := appendScan(ctx, tx, keyID, back, models.ActionReturn, "скан QR: возврат"); err != nil {
			return nil, err
		}
		out.Action = models.ActionReturn
		out.HolderName = back.Name
		return out, tx.Commit()
	}

	// Сканировал другой человек — закрываем прошлую выдачу и оформляем новую.
	if actor.Intent == IntentReturn {
		return nil, ErrNotHolder
	}
	if guest && !guestNamed {
		return nil, ErrGuestDataNeeded
	}
	if err := s.setKeyStatus(ctx, tx, keyID, models.KeyStatusIssued, models.KeyStatusIssued); err != nil {
		return nil, err
	}
	if holderKnown {
		if err := appendScan(ctx, tx, keyID, actorFromLog(holder), models.ActionReturn, "скан QR: передача ключа"); err != nil {
			return nil, err
		}
	}
	if err := appendScan(ctx, tx, keyID, actor, models.ActionIssue, "скан QR: приём ключа"); err != nil {
		return nil, err
	}
	out.Action = models.ActionIssue
	out.HolderName = actor.Name
	out.Transferred = holderKnown
	return out, tx.Commit()
}

// HolderInfo возвращает текущего держателя ключа; nil — если ключ свободен,
// утерян или держатель неизвестен (записи старого формата).
func (s *KeyService) HolderInfo(ctx context.Context, keyID int64) (*models.KeyLog, error) {
	var status models.KeyStatus
	if err := s.db.GetContext(ctx, &status, `SELECT status FROM keys WHERE id = ?`, keyID); err != nil {
		return nil, fmt.Errorf("key %d not found", keyID)
	}
	if status != models.KeyStatusIssued {
		return nil, nil
	}

	holder := &models.KeyLog{}
	err := s.db.GetContext(ctx, holder,
		`SELECT id, key_id, user_id, action_type, timestamp, comment, guest_name, guest_phone, guest_token
		 FROM key_logs WHERE key_id = ? AND action_type = ?
		 ORDER BY id DESC LIMIT 1`, keyID, models.ActionIssue)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("current holder: %w", err)
	}
	return holder, nil
}

// setKeyStatus меняет статус ключа только из ожидаемого состояния: защита от
// двух одновременных сканов одного и того же QR.
func (s *KeyService) setKeyStatus(ctx context.Context, tx *sqlx.Tx, keyID int64, from, to models.KeyStatus) error {
	res, err := tx.ExecContext(ctx, `UPDATE keys SET status = ? WHERE id = ? AND status = ?`, to, keyID, from)
	if err != nil {
		return fmt.Errorf("update key status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrScanConflict
	}
	return nil
}

// appendScan пишет событие сканирования: сотрудник — по user_id, гость — по
// ФИО, телефону и метке браузера.
func appendScan(ctx context.Context, tx *sqlx.Tx, keyID int64, actor ScanActor, action models.ActionType, comment string) error {
	entry := &models.KeyLog{
		KeyID:      keyID,
		UserID:     optional(actor.UserID),
		ActionType: action,
		Timestamp:  time.Now(),
		Comment:    optional(comment),
	}
	if actor.UserID == "" {
		entry.GuestName = optional(actor.Name)
		entry.GuestPhone = optional(actor.Phone)
		entry.GuestToken = optional(actor.Token)
	}
	if _, err := tx.NamedExecContext(ctx,
		`INSERT INTO key_logs (key_id, user_id, action_type, timestamp, comment, guest_name, guest_phone, guest_token)
		 VALUES (:key_id, :user_id, :action_type, :timestamp, :comment, :guest_name, :guest_phone, :guest_token)`, entry); err != nil {
		return fmt.Errorf("insert scan log: %w", err)
	}
	return nil
}

func actorFromLog(l *models.KeyLog) ScanActor {
	if l == nil {
		return ScanActor{}
	}
	return ScanActor{UserID: deref(l.UserID), Name: deref(l.GuestName), Phone: deref(l.GuestPhone), Token: deref(l.GuestToken)}
}

// matches — скан сделан тем же человеком, что и предыдущая выдача: для
// сотрудника сверяем аккаунт, для гостя — метку браузера или номер телефона.
func (a ScanActor) matches(l *models.KeyLog) bool {
	if l == nil {
		return false
	}
	if a.UserID != "" {
		return l.UserID != nil && *l.UserID == a.UserID
	}
	if l.UserID != nil {
		return false
	}
	if a.Token != "" && l.GuestToken != nil && *l.GuestToken == a.Token {
		return true
	}
	return samePhone(a.Phone, deref(l.GuestPhone))
}

func samePhone(a, b string) bool {
	digits := func(s string) string {
		var out []rune
		for _, r := range s {
			if r >= '0' && r <= '9' {
				out = append(out, r)
			}
		}
		return string(out)
	}
	x, y := digits(a), digits(b)
	if x == "" || y == "" {
		return false
	}
	if len(x) > 10 {
		x = x[len(x)-10:]
	}
	if len(y) > 10 {
		y = y[len(y)-10:]
	}
	return x == y
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optional(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
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
