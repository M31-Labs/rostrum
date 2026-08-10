package present

import "testing"

func TestAllowedURLAcceptsOnlyHTTPAndHTTPS(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"https://example.com/maya", true},
		{"http://example.com", true},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>alert(1)</script>", false},
		{"vbscript:msgbox(1)", false},
		{"ftp://example.com/file", false},
		{"not a url at all", false},
		{"//example.com/protocol-relative", false},
		{"https://", false},
	}
	for _, test := range cases {
		if got := AllowedURL(test.raw); got != test.want {
			t.Errorf("AllowedURL(%q) = %v, want %v", test.raw, got, test.want)
		}
	}
}

func TestCSVSafeNeutralizesFormulaPrefixes(t *testing.T) {
	cases := []struct {
		cell string
		want string
	}{
		{"", ""},
		{"Ordinary title", "Ordinary title"},
		{`=HYPERLINK("https://evil.example")`, `'=HYPERLINK("https://evil.example")`},
		{"+1 (555) 555-0100", "'+1 (555) 555-0100"},
		{"-5", "'-5"},
		{"@mention", "'@mention"},
		{"\ttabbed", "'\ttabbed"},
		{"\rcarriage", "'\rcarriage"},
	}
	for _, test := range cases {
		if got := CSVSafe(test.cell); got != test.want {
			t.Errorf("CSVSafe(%q) = %q, want %q", test.cell, got, test.want)
		}
	}
}
