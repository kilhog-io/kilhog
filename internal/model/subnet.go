package model

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	IPv4HostPrefix = 32
	IPv6HostPrefix = 128
)

// Subnet represents an IP address space (CIDR block or host address).
type Subnet struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Prefix      int         `json:"prefix"`
	Address     string      `json:"address"`
	Type        AddressType `json:"type"`
	Parent      Parent      `json:"parent"`
	Tags        []Tag       `json:"tags,omitempty"`
}

// CIDR returns the subnet notation built from address and prefix.
func (s Subnet) CIDR() string {
	return fmt.Sprintf("%s/%d", s.Address, s.Prefix)
}

// IsLeaf reports whether the subnet represents a single host address.
func (s Subnet) IsLeaf() bool {
	switch s.Type {
	case AddressTypeIPv4:
		return s.Prefix == IPv4HostPrefix
	case AddressTypeIPv6:
		return s.Prefix == IPv6HostPrefix
	default:
		return false
	}
}
