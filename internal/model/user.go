package model

import (
	"time"

	"github.com/google/uuid"
)

// UserRole is a local user's authorization role for identity administration.
type UserRole string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

// LocalUser is an account whose credentials are managed by kilhog.
type LocalUser struct {
	UUID           uuid.UUID `json:"uuid"`
	Username       string    `json:"username"`
	PasswordHash   string    `json:"-"`
	DisplayName    string    `json:"display_name,omitempty"`
	Email          string    `json:"email,omitempty"`
	Role           UserRole  `json:"role"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
