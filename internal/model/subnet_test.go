package model

import "testing"

func TestSubnet_CIDR(t *testing.T) {
	tests := []struct {
		name    string
		subnet  Subnet
		want    string
	}{
		{
			name: "ipv4 block",
			subnet: Subnet{
				Address: "192.168.1.0",
				Prefix:  24,
				Type:    AddressTypeIPv4,
			},
			want: "192.168.1.0/24",
		},
		{
			name: "ipv4 host",
			subnet: Subnet{
				Address: "192.168.1.42",
				Prefix:  32,
				Type:    AddressTypeIPv4,
			},
			want: "192.168.1.42/32",
		},
		{
			name: "ipv6 host",
			subnet: Subnet{
				Address: "2001:db8::1",
				Prefix:  128,
				Type:    AddressTypeIPv6,
			},
			want: "2001:db8::1/128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.subnet.CIDR(); got != tt.want {
				t.Errorf("Subnet.CIDR() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubnet_IsLeaf(t *testing.T) {
	tests := []struct {
		name   string
		subnet Subnet
		want   bool
	}{
		{
			name: "ipv4 block is not leaf",
			subnet: Subnet{
				Prefix: 24,
				Type:   AddressTypeIPv4,
			},
			want: false,
		},
		{
			name: "ipv4 host is leaf",
			subnet: Subnet{
				Prefix: 32,
				Type:   AddressTypeIPv4,
			},
			want: true,
		},
		{
			name: "ipv6 block is not leaf",
			subnet: Subnet{
				Prefix: 64,
				Type:   AddressTypeIPv6,
			},
			want: false,
		},
		{
			name: "ipv6 host is leaf",
			subnet: Subnet{
				Prefix: 128,
				Type:   AddressTypeIPv6,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.subnet.IsLeaf(); got != tt.want {
				t.Errorf("Subnet.IsLeaf() = %v, want %v", got, tt.want)
			}
		})
	}
}
