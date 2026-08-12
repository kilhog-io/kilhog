package db

import "testing"

func TestParseDialect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw     string
		want    Dialect
		wantErr bool
	}{
		{raw: "sqlite", want: DialectSQLite},
		{raw: "postgres", want: DialectPostgres},
		{raw: "d1", want: DialectD1},
		{raw: "mysql", wantErr: true},
	}

	for _, tc := range cases {
		got, err := ParseDialect(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseDialect(%q) error = nil, want error", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseDialect(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("ParseDialect(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestDialectHelpers(t *testing.T) {
	t.Parallel()

	if !DialectSQLite.UsesSQLiteSyntax() || !DialectD1.UsesSQLiteSyntax() {
		t.Fatal("sqlite and d1 should use SQLite syntax")
	}
	if DialectPostgres.UsesSQLiteSyntax() {
		t.Fatal("postgres should not use SQLite syntax")
	}
	if DialectD1.SupportsSQLTransactions() {
		t.Fatal("d1 should not report SQL transaction support")
	}
	if DialectD1.MigrationDialect() != string(DialectSQLite) {
		t.Fatalf("d1 migrations should reuse sqlite, got %q", DialectD1.MigrationDialect())
	}
}
