package config

import "testing"

func TestCabinetBaseURL(t *testing.T) {
	tests := []struct {
		name, publicURL, subdomain, want string
		invalid                          bool
	}{
		{name: "same domain", publicURL: "https://example.com", want: ""},
		{name: "separate cabinet", publicURL: "https://example.com", subdomain: "my", want: "https://my.example.com"},
		{name: "case normalization", publicURL: "https://example.com", subdomain: "My-Account", want: "https://my-account.example.com"},
		{name: "full hostname rejected", publicURL: "https://example.com", subdomain: "my.example.com", invalid: true},
		{name: "leading hyphen rejected", publicURL: "https://example.com", subdomain: "-my", invalid: true},
		{name: "domain required", subdomain: "my", invalid: true},
		{name: "path rejected", publicURL: "https://example.com/app", subdomain: "my", invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cabinetBaseURL(test.publicURL, test.subdomain)
			if (err != nil) != test.invalid || got != test.want {
				t.Fatalf("cabinetBaseURL(%q, %q) = %q, %v; want %q, invalid=%v", test.publicURL, test.subdomain, got, err, test.want, test.invalid)
			}
		})
	}
}
