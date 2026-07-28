package model

import "github.com/google/uuid"

// Network is the root tenancy container for subnets.
type Network struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []Tag     `json:"tags,omitempty"`
}
