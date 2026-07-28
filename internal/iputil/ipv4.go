package iputil

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/kilhog-io/kilhog/internal/model"
)

var (
	ErrInvalidIPv4Address = errors.New("invalid IPv4 address")
	ErrInvalidPrefix      = errors.New("invalid IPv4 prefix length")
	ErrPrefixTooBroad     = errors.New("subnet prefix is less specific than parent")
	ErrOutsideParent      = errors.New("subnet is outside parent CIDR")
	ErrOverlap            = errors.New("subnet overlaps with sibling")
	ErrNoFreeBlock        = errors.New("no free address block found in parent")
)

// ParseIPv4Prefix builds a normalized IPv4 prefix from address and prefix length.
func ParseIPv4Prefix(address string, prefixLen int) (netip.Prefix, error) {
	if prefixLen < 1 || prefixLen > model.IPv4HostPrefix {
		return netip.Prefix{}, ErrInvalidPrefix
	}

	addr, err := netip.ParseAddr(address)
	if err != nil || !addr.Is4() {
		return netip.Prefix{}, ErrInvalidIPv4Address
	}

	prefix := netip.PrefixFrom(addr, prefixLen)
	if !prefix.IsValid() {
		return netip.Prefix{}, ErrInvalidPrefix
	}

	return prefix.Masked(), nil
}

// PrefixContains reports whether child is fully contained within parent.
func PrefixContains(parent, child netip.Prefix) bool {
	if child.Bits() < parent.Bits() {
		return false
	}
	return parent.Contains(child.Addr())
}

// ValidateIPv4Subnet checks prefix bounds, parent containment, and sibling overlap.
func ValidateIPv4Subnet(candidate netip.Prefix, parent *netip.Prefix, siblings []netip.Prefix) error {
	if candidate.Bits() < 1 || candidate.Bits() > model.IPv4HostPrefix {
		return ErrInvalidPrefix
	}

	if parent != nil {
		if candidate.Bits() < parent.Bits() {
			return ErrPrefixTooBroad
		}
		if !PrefixContains(*parent, candidate) {
			return ErrOutsideParent
		}
	}

	for _, sibling := range siblings {
		if candidate.Overlaps(sibling) {
			return ErrOverlap
		}
	}

	return nil
}

// FindFreeIPv4Block locates a non-overlapping prefix within a subnet parent CIDR.
func FindFreeIPv4Block(parent netip.Prefix, prefixLen int, siblings []netip.Prefix) (netip.Prefix, error) {
	if prefixLen < 1 || prefixLen > model.IPv4HostPrefix {
		return netip.Prefix{}, ErrInvalidPrefix
	}

	if prefixLen < parent.Bits() {
		return netip.Prefix{}, ErrPrefixTooBroad
	}
	if block, ok := findFreeInRange(parent, prefixLen, siblings); ok {
		return block, nil
	}
	return netip.Prefix{}, ErrNoFreeBlock
}

func findFreeInRange(searchSpace netip.Prefix, prefixLen int, siblings []netip.Prefix) (netip.Prefix, bool) {
	if prefixLen < searchSpace.Bits() {
		return netip.Prefix{}, false
	}

	start := addrToUint32(searchSpace.Addr())
	end := addrToUint32(lastAddr(searchSpace))
	step := uint32(1) << (32 - prefixLen)
	mask := ^uint32(0) << (32 - prefixLen)

	candidate := start & mask
	if candidate < start {
		candidate += step
	}

	for ; candidate <= end; candidate += step {
		addr := uint32ToAddr(candidate)
		if !searchSpace.Contains(addr) {
			continue
		}

		block := netip.PrefixFrom(addr, prefixLen).Masked()
		overlaps := false
		for _, sibling := range siblings {
			if block.Overlaps(sibling) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			return block, true
		}
	}

	return netip.Prefix{}, false
}

func lastAddr(prefix netip.Prefix) netip.Addr {
	start := addrToUint32(prefix.Addr())
	hostBits := 32 - prefix.Bits()
	if hostBits >= 32 {
		return prefix.Addr()
	}
	return uint32ToAddr(start + (1 << hostBits) - 1)
}

func addrToUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func uint32ToAddr(value uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	})
}

// PrefixAddress returns the canonical string form of a prefix network address.
func PrefixAddress(prefix netip.Prefix) string {
	return prefix.Addr().String()
}

// PrefixString returns the CIDR notation for a prefix.
func PrefixString(prefix netip.Prefix) string {
	return fmt.Sprintf("%s/%d", prefix.Addr(), prefix.Bits())
}
