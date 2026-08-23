package store

import (
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/tespio/go-rdap-server/internal/config"
)

func TestIPRangeV4(t *testing.T) {
	tests := []struct {
		cidr          string
		start, end    uint64
	}{
		{"8.8.8.0/24", 0x08080800, 0x080808ff},
		{"1.0.0.0/24", 0x01000000, 0x010000ff},
		{"192.168.1.5/32", 0xc0a80105, 0xc0a80105},
		{"10.0.0.0/8", 0x0a000000, 0x0affffff},
		{"0.0.0.0/0", 0x00000000, 0xffffffff},
	}

	for _, tt := range tests {
		version, start, end, start6, end6, err := ipRange(tt.cidr)
		if err != nil {
			t.Fatalf("ipRange(%q): unexpected error: %v", tt.cidr, err)
		}
		if version != "v4" {
			t.Errorf("ipRange(%q): version = %q, want v4", tt.cidr, version)
		}
		if start != tt.start || end != tt.end {
			t.Errorf("ipRange(%q): got [%d,%d], want [%d,%d]", tt.cidr, start, end, tt.start, tt.end)
		}
		if start6 != nil || end6 != nil {
			t.Errorf("ipRange(%q): expected nil v6 bounds", tt.cidr)
		}
	}
}

func TestIPRangeV6(t *testing.T) {
	tests := []struct {
		cidr       string
		start, end string // hex
	}{
		{"2001:4860::/32", "20014860000000000000000000000000", "20014860ffffffffffffffffffffffff"},
		{"2606:4700:4700::1111/128", "26064700470000000000000000001111", "26064700470000000000000000001111"},
		{"::/0", "00000000000000000000000000000000", "ffffffffffffffffffffffffffffffff"},
		{"2001:db8::/120", "20010db8000000000000000000000000", "20010db80000000000000000000000ff"},
	}

	for _, tt := range tests {
		version, start, end, start6, end6, err := ipRange(tt.cidr)
		if err != nil {
			t.Fatalf("ipRange(%q): unexpected error: %v", tt.cidr, err)
		}
		if version != "v6" {
			t.Errorf("ipRange(%q): version = %q, want v6", tt.cidr, version)
		}
		if start != 0 || end != 0 {
			t.Errorf("ipRange(%q): expected zero v4 bounds", tt.cidr)
		}
		if hexBytes(start6) != tt.start {
			t.Errorf("ipRange(%q): start = %s, want %s", tt.cidr, hexBytes(start6), tt.start)
		}
		if hexBytes(end6) != tt.end {
			t.Errorf("ipRange(%q): end = %s, want %s", tt.cidr, hexBytes(end6), tt.end)
		}
	}
}

func TestIPRangeInvalid(t *testing.T) {
	if _, _, _, _, _, err := ipRange("not-a-cidr"); err == nil {
		t.Error("ipRange(invalid): expected error")
	}
	if _, _, _, _, _, err := ipRange("8.8.8.8/33"); err == nil {
		t.Error("ipRange(8.8.8.8/33): expected error")
	}
}

func hexBytes(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexDigits[v>>4]
		out[i*2+1] = hexDigits[v&0xf]
	}
	return string(out)
}

func TestHexBytes(t *testing.T) {
	if got := hexBytes([]byte{0x20, 0x01}); got != "2001" {
		t.Fatalf("hexBytes: got %s", got)
	}
}

func TestNormalizeMySQLDSN(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "native DSN passes through",
			in:   "rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4",
			want: "rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4",
		},
		{
			name: "mysql URL",
			in:   "mysql://rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4",
			want: "rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4",
		},
		{
			name: "mysql URL without tcp()",
			in:   "mysql://user:pass@db.example.com:3307/mydb?charset=utf8mb4&timeout=5s",
			want: "user:pass@tcp(db.example.com:3307)/mydb?charset=utf8mb4&timeout=5s",
		},
		{
			name: "mysql URL with no password",
			in:   "mysql://root@localhost/rdap",
			want: "root@tcp(localhost)/rdap",
		},
		{
			name: "mysql URL with no credentials",
			in:   "mysql://localhost/rdap",
			want: "tcp(localhost)/rdap",
		},
		{
			name:    "invalid URL",
			in:      "mysql://%zz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMySQLDSN(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMySQLDSNParseable(t *testing.T) {
	// Ensure the URL form produces a DSN that go-sql-driver can parse.
	dsn, err := normalizeMySQLDSN("mysql://rdap:rdap@tcp(localhost:3306)/rdap?parseTime=true&charset=utf8mb4")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q): %v", dsn, err)
	}
	if !mc.ParseTime {
		t.Error("expected ParseTime=true")
	}
	if mc.Addr != "localhost:3306" {
		t.Errorf("Addr = %q, want localhost:3306", mc.Addr)
	}
	if mc.DBName != "rdap" {
		t.Errorf("DBName = %q, want rdap", mc.DBName)
	}
	if mc.User != "rdap" || mc.Passwd != "rdap" {
		t.Errorf("credentials = %q:%q, want rdap:rdap", mc.User, mc.Passwd)
	}
}

func TestNewMySQLStoreInvalidDSN(t *testing.T) {
	// An unparseable DSN fails fast, before any network connection is attempted.
	if _, err := NewMySQLStore(config.StorageConfig{DSN: "not-a-dsn@@"}); err == nil {
		t.Error("expected error for invalid DSN")
	}
	// Empty driver DSN also fails fast.
	if _, err := NewMySQLStore(config.StorageConfig{DSN: ""}); err == nil {
		t.Error("expected error for empty DSN")
	}
}