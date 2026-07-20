package utils

import (
	"testing"
)

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected *string
	}{
		{
			name:     "valid username",
			input:    stringPtr("john_doe"),
			expected: stringPtr("john_doe"),
		},
		{
			name:     "username with telegram url",
			input:    stringPtr("t.me/spam"),
			expected: nil,
		},
		{
			name:     "username with obfuscated telegram",
			input:    stringPtr("t.•m•e"),
			expected: nil,
		},
		{
			name:     "username with service word",
			input:    stringPtr("telegram_bot"),
			expected: nil,
		},
		{
			name:     "nil username",
			input:    nil,
			expected: nil,
		},
		{
			name:     "username with @",
			input:    stringPtr("@john_doe"),
			expected: stringPtr("john_doe"),
		},
		{
			name:     "valid username with numbers",
			input:    stringPtr("ioajfd123"),
			expected: stringPtr("ioajfd123"),
		},
		{
			name:     "valid username with @ and numbers",
			input:    stringPtr("@ioajfd123"),
			expected: stringPtr("ioajfd123"),
		},
		{
			name:     "valid username with www substring",
			input:    stringPtr("tsstewww"),
			expected: stringPtr("tsstewww"),
		},
		{
			name:     "valid username with @ and www substring",
			input:    stringPtr("@tsstewww"),
			expected: stringPtr("tsstewww"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeUsername(tt.input)
			if !equalStringPtr(result, tt.expected) {
				t.Errorf("SanitizeUsername(%v) = %v, want %v",
					ptrToString(tt.input), ptrToString(result), ptrToString(tt.expected))
			}
		})
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected *string
	}{
		{
			name:     "valid display name",
			input:    stringPtr("John"),
			expected: stringPtr("John"),
		},
		{
			name:     "valid Russian name Alexey",
			input:    stringPtr("Алексей"),
			expected: stringPtr("Алексей"),
		},
		{
			name:     "valid display name with special chars",
			input:    stringPtr("$_"),
			expected: stringPtr("$"),
		},
		{
			name:     "display name with URL",
			input:    stringPtr("John https://t.me/spam"),
			expected: nil,
		},
		{
			name:     "display name with service word",
			input:    stringPtr("Telegram Support"),
			expected: nil,
		},
		{
			name:     "display name with Russian service word",
			input:    stringPtr("Телеграм Поддержка"),
			expected: nil,
		},
		{
			name:     "nil display name",
			input:    nil,
			expected: nil,
		},
		{
			name:     "display name with @",
			input:    stringPtr("John @user"),
			expected: stringPtr("John user"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeDisplayName(tt.input)
			if !equalStringPtr(result, tt.expected) {
				t.Errorf("SanitizeDisplayName(%v) = %v, want %v",
					ptrToString(tt.input), ptrToString(result), ptrToString(tt.expected))
			}
		})
	}
}

func TestIsSuspiciousUser(t *testing.T) {
	tests := []struct {
		name      string
		username  *string
		firstName *string
		lastName  *string
		expected  bool
	}{
		{
			name:      "normal user",
			username:  stringPtr("john_doe"),
			firstName: stringPtr("John"),
			lastName:  stringPtr("Doe"),
			expected:  false,
		},
		{
			name:      "normal Russian user Alexey",
			username:  stringPtr("alexey123"),
			firstName: stringPtr("Алексей"),
			lastName:  stringPtr("Иванов"),
			expected:  false,
		},
		{
			name:      "Russian user Alexey with nil lastname",
			username:  stringPtr("user123"),
			firstName: stringPtr("Алексей"),
			lastName:  nil,
			expected:  false,
		},
		{
			name:      "Russian user Alexey with empty string lastname",
			username:  stringPtr("user456"),
			firstName: stringPtr("Алексей"),
			lastName:  stringPtr(""),
			expected:  false,
		},
		{
			name:      "valid user tsstewww with special chars firstName",
			username:  stringPtr("tsstewww"),
			firstName: stringPtr("$_"),
			lastName:  nil,
			expected:  false,
		},
		{
			name:      "valid user seleqep with dot firstName",
			username:  stringPtr("seleqep"),
			firstName: stringPtr("."),
			lastName:  nil,
			expected:  false,
		},
		{
			name:      "username with telegram",
			username:  stringPtr("telegram_bot"),
			firstName: stringPtr("John"),
			lastName:  stringPtr("Doe"),
			expected:  true,
		},
		{
			name:      "first name with service word",
			username:  stringPtr("john_doe"),
			firstName: stringPtr("Telegram Support"),
			lastName:  nil,
			expected:  true,
		},
		{
			name:      "last name with URL",
			username:  stringPtr("john_doe"),
			firstName: stringPtr("John"),
			lastName:  stringPtr("t.me/spam"),
			expected:  true,
		},
		{
			name:      "username with obfuscated domain",
			username:  stringPtr("t•m•e"),
			firstName: nil,
			lastName:  nil,
			expected:  true,
		},
		{
			name:      "Russian service name",
			username:  nil,
			firstName: stringPtr("Телеграм"),
			lastName:  nil,
			expected:  true,
		},
		{
			name:      "all nil",
			username:  nil,
			firstName: nil,
			lastName:  nil,
			expected:  false,
		},
		{
			name:      "valid user @CompanySupportAdmin - support alone is not suspicious",
			username:  stringPtr("CompanySupportAdmin"),
			firstName: stringPtr("Company"),
			lastName:  stringPtr("Admin"),
			expected:  false,
		},
		{
			name:      "suspicious user with telegram and support combination",
			username:  stringPtr("TelegramSupport"),
			firstName: stringPtr("Support"),
			lastName:  nil,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSuspiciousUser(tt.username, tt.firstName, tt.lastName)
			if result != tt.expected {
				t.Errorf("IsSuspiciousUser(%v, %v, %v) = %v, want %v",
					ptrToString(tt.username), ptrToString(tt.firstName),
					ptrToString(tt.lastName), result, tt.expected)
			}
		})
	}
}

func TestUsernameForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		username *string
		withAt   bool
		expected string
	}{
		{
			name:     "valid username with @",
			username: stringPtr("john_doe"),
			withAt:   true,
			expected: "@john_doe",
		},
		{
			name:     "valid username without @",
			username: stringPtr("john_doe"),
			withAt:   false,
			expected: "john_doe",
		},
		{
			name:     "suspicious username",
			username: stringPtr("telegram"),
			withAt:   true,
			expected: "клиент",
		},
		{
			name:     "nil username",
			username: nil,
			withAt:   false,
			expected: "клиент",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UsernameForDisplay(tt.username, tt.withAt)
			if result != tt.expected {
				t.Errorf("UsernameForDisplay(%v, %v) = %v, want %v",
					ptrToString(tt.username), tt.withAt, result, tt.expected)
			}
		})
	}
}

func TestDisplayNameOrFallback(t *testing.T) {
	tests := []struct {
		name      string
		firstName *string
		fallback  string
		expected  string
	}{
		{
			name:      "valid first name",
			firstName: stringPtr("John"),
			fallback:  "Unknown",
			expected:  "John",
		},
		{
			name:      "suspicious first name with fallback",
			firstName: stringPtr("Telegram"),
			fallback:  "User123",
			expected:  "User123",
		},
		{
			name:      "nil first name with fallback",
			firstName: nil,
			fallback:  "Guest",
			expected:  "Guest",
		},
		{
			name:      "nil first name without fallback",
			firstName: nil,
			fallback:  "",
			expected:  "клиент",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DisplayNameOrFallback(tt.firstName, tt.fallback)
			if result != tt.expected {
				t.Errorf("DisplayNameOrFallback(%v, %v) = %v, want %v",
					ptrToString(tt.firstName), tt.fallback, result, tt.expected)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func ptrToString(ptr *string) string {
	if ptr == nil {
		return "<nil>"
	}
	return *ptr
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
