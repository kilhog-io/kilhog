package model

// AddressType is the address family of a subnet.
type AddressType string

const (
	AddressTypeIPv4 AddressType = "ipv4"
	AddressTypeIPv6 AddressType = "ipv6"
)
