package model

import "github.com/google/uuid"

// ParentKind identifies whether a subnet parent is a network or another subnet.
type ParentKind string

const (
	ParentKindNetwork ParentKind = "network"
	ParentKindSubnet  ParentKind = "subnet"
)

// Parent is a reference to the parent of a subnet in the hierarchy.
type Parent struct {
	Kind ParentKind `json:"kind"`
	UUID uuid.UUID  `json:"uuid"`
}
