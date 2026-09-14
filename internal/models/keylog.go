package models

import "time"

type ActionType string

const (
	ActionIssue   ActionType = "issue"
	ActionReturn  ActionType = "return"
	ActionLost    ActionType = "lost"
	ActionRestore ActionType = "restore"
)

// KeyLog — событие журнала ключа. UserID пуст, когда ключ взял гость без
// аккаунта: тогда заполнены GuestName и GuestPhone, а GuestToken отмечает
// браузер, с которого гость сканировал QR (по нему узнаём его при сдаче).
type KeyLog struct {
	ID         int64      `json:"id" db:"id"`
	KeyID      int64      `json:"key_id" db:"key_id"`
	UserID     *string    `json:"user_id" db:"user_id"`
	ActionType ActionType `json:"action_type" db:"action_type"`
	Timestamp  time.Time  `json:"timestamp" db:"timestamp"`
	Comment    *string    `json:"comment,omitempty" db:"comment"`
	GuestName  *string    `json:"guest_name,omitempty" db:"guest_name"`
	GuestPhone *string    `json:"guest_phone,omitempty" db:"guest_phone"`
	GuestToken *string    `json:"guest_token,omitempty" db:"guest_token"`
}
