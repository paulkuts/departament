package models

type KeyStatus string

const (
	KeyStatusAvailable KeyStatus = "available"
	KeyStatusIssued    KeyStatus = "issued"
	KeyStatusLost      KeyStatus = "lost"
)

type Key struct {
	ID              int64     `json:"id" db:"id"`
	KeyNumber       string    `json:"key_number" db:"key_number"`
	RoomDescription string    `json:"room_description" db:"room_description"`
	Status          KeyStatus `json:"status" db:"status"`
	Notes           *string   `json:"notes,omitempty" db:"notes"`
}
