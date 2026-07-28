package iputil_test

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/kilhog-io/kilhog/internal/iputil"
)

func TestParseIPv4Prefix(t *testing.T) {
	tests := []struct {
		name    string
		address string
		prefix  int
		want    string
		wantErr error
	}{
		{
			name:    "normalized network address",
			address: "192.168.1.5",
			prefix:  24,
			want:    "192.168.1.0/24",
		},
		{
			name:    "host address",
			address: "192.168.1.42",
			prefix:  32,
			want:    "192.168.1.42/32",
		},
		{
			name:    "invalid address",
			address: "not-an-ip",
			prefix:  24,
			wantErr: iputil.ErrInvalidIPv4Address,
		},
		{
			name:    "invalid prefix",
			address: "10.0.0.0",
			prefix:  33,
			wantErr: iputil.ErrInvalidPrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := iputil.ParseIPv4Prefix(tt.address, tt.prefix)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseIPv4Prefix() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseIPv4Prefix() error = %v", err)
			}
			if iputil.PrefixString(got) != tt.want {
				t.Fatalf("prefix = %q, want %q", iputil.PrefixString(got), tt.want)
			}
		})
	}
}

func TestValidateIPv4Subnet(t *testing.T) {
	parent := netip.MustParsePrefix("192.168.0.0/16")
	siblings := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
	}

	tests := []struct {
		name    string
		cidr    string
		wantErr error
	}{
		{name: "valid child", cidr: "192.168.2.0/24"},
		{name: "overlap sibling", cidr: "192.168.1.0/24", wantErr: iputil.ErrOverlap},
		{name: "partial overlap", cidr: "192.168.1.128/25", wantErr: iputil.ErrOverlap},
		{name: "outside parent", cidr: "10.0.0.0/24", wantErr: iputil.ErrOutsideParent},
		{name: "broader than parent", cidr: "192.168.0.0/8", wantErr: iputil.ErrPrefixTooBroad},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := netip.MustParsePrefix(tt.cidr)
			err := iputil.ValidateIPv4Subnet(candidate, &parent, siblings)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateIPv4Subnet() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateIPv4Subnet() error = %v", err)
			}
		})
	}
}

func TestFindFreeIPv4Block(t *testing.T) {
	parent := netip.MustParsePrefix("192.168.0.0/16")
	siblings := []netip.Prefix{
		netip.MustParsePrefix("192.168.0.0/24"),
		netip.MustParsePrefix("192.168.1.0/24"),
	}

	block, err := iputil.FindFreeIPv4Block(parent, 24, siblings)
	if err != nil {
		t.Fatalf("FindFreeIPv4Block() error = %v", err)
	}
	if block.String() != "192.168.2.0/24" {
		t.Fatalf("block = %q, want 192.168.2.0/24", block)
	}
}

func TestFindFreeIPv4BlockNoSpace(t *testing.T) {
	parent := netip.MustParsePrefix("192.168.0.0/24")
	siblings := []netip.Prefix{parent}

	_, err := iputil.FindFreeIPv4Block(parent, 24, siblings)
	if !errors.Is(err, iputil.ErrNoFreeBlock) {
		t.Fatalf("FindFreeIPv4Block() error = %v, want ErrNoFreeBlock", err)
	}
}
