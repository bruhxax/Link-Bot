package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"strings"
	"testing"
)

func TestAISecretsAreEncryptedAndNotExposedInCatalog(t *testing.T) {
	block, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{aead: aead, records: map[string]record{ProviderSupportAI: {Enabled: true, Config: map[string]string{"apiUrl": "https://provider.example/v1", "apiKey": "private-test-key", "model": "model", "prompt": "private admin instructions"}}}}
	fields, enabled := s.SupportAISettings()
	if !enabled {
		t.Fatal("valid support AI settings are disabled")
	}
	encrypted, err := s.encrypt(fields)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "private-test-key") {
		t.Fatal("key stored in plain text")
	}
	decoded, err := s.decrypt(encrypted)
	if err != nil || decoded["apiKey"] != "private-test-key" {
		t.Fatalf("cannot restore encrypted key: %v", err)
	}
	raw, _ := json.Marshal(s.ListAdmin())
	if strings.Contains(string(raw), "private-test-key") || strings.Contains(string(raw), "private admin instructions") || strings.Contains(string(raw), ProviderSupportAI) {
		t.Fatal("support AI exposed in general integration catalog")
	}
	fields["apiKey"] = "modified"
	stored, _ := s.SupportAISettings()
	if stored["apiKey"] != "private-test-key" {
		t.Fatal("caller mutated stored credentials")
	}
}

func TestInvalidAIUpdateDoesNotMutateSavedCredentials(t *testing.T) {
	s := &Service{records: map[string]record{ProviderSupportAI: {Config: map[string]string{"apiKey": "original"}}}}
	_, err := s.Update(context.Background(), ProviderSupportAI, UpdateInput{Enabled: true, Fields: map[string]string{"apiKey": "replacement", "unknown": "field"}}, 1)
	if err == nil {
		t.Fatal("accepted invalid update")
	}
	fields, _ := s.SupportAISettings()
	if fields["apiKey"] != "original" {
		t.Fatal("failed update changed credentials")
	}
}
