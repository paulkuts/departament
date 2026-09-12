package models

import "time"

type ActionType string

const (
	ActionIssue  ActionType = "issue"
	ActionReturn ActionType = "return"
)

type KeyLog struct {
	ID         int64      `json:"id" db:"id"`
	KeyID      int64      `json:"key_id" db:"key_id"`
	UserID     string     `json:"user_id" db:"user_id"`
	ActionType ActionType `json:"action_type" db:"action_type"`
	Timestamp  time.Time  `json:"timestamp" db:"timestamp"`
	Comment    *string    `json:"comment,omitempty" db:"comment"`
}
