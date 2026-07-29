package log

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		raw  string
		want Level
	}{
		{"", LevelInfo},
		{"info", LevelInfo},
		{"DEBUG", LevelDebug},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"off", LevelOff},
		{"none", LevelOff},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.raw)
		if err != nil {
			t.Fatalf("ParseLevel(%q) error = %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}

	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel(verbose) expected error")
	}
}

func TestEnabled(t *testing.T) {
	SetLevel(LevelInfo)
	if !Enabled(LevelInfo) {
		t.Fatal("info should be enabled at info level")
	}
	if Enabled(LevelDebug) {
		t.Fatal("debug should not be enabled at info level")
	}

	SetLevel(LevelDebug)
	if !Enabled(LevelDebug) {
		t.Fatal("debug should be enabled at debug level")
	}

	SetLevel(LevelOff)
	if Enabled(LevelError) {
		t.Fatal("error should not be enabled at off level")
	}

	SetLevel(LevelInfo)
}
