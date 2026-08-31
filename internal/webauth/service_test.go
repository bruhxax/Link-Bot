package webauth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChallengeApprovalAndSingleUseConsumption(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(time.Minute)

	challenge, err := service.Create(now)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if challenge.ID == "" || challenge.Secret == "" || challenge.ApprovalToken == "" {
		t.Fatal("challenge credentials must not be empty")
	}
	if challenge.ID == challenge.Secret || challenge.ID == challenge.ApprovalToken || challenge.Secret == challenge.ApprovalToken {
		t.Fatal("challenge credentials must be independent")
	}
	if parameter := StartParameter(challenge.ApprovalToken); len(parameter) > 64 || !strings.HasPrefix(parameter, StartParameterPrefix) {
		t.Fatalf("invalid Telegram start parameter: %q", parameter)
	}
	if _, err := service.Consume(challenge.ID, challenge.Secret, now); !errors.Is(err, ErrPending) {
		t.Fatalf("Consume before approval error = %v, want ErrPending", err)
	}
	if _, err := service.Consume(challenge.ID, "wrong", now); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("Consume with wrong secret error = %v, want ErrInvalidSecret", err)
	}

	want := TelegramUser{ID: 42, FirstName: "Max", Username: "max", LanguageCode: "ru"}
	if err := service.Approve(challenge.ApprovalToken, want, now.Add(time.Second)); err != nil {
		t.Fatalf("Approve returned error: %v", err)
	}
	if err := service.Approve(challenge.ApprovalToken, want, now.Add(2*time.Second)); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second Approve error = %v, want ErrAlreadyClaimed", err)
	}

	got, err := service.Consume(challenge.ID, challenge.Secret, now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("Consume returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Consume user = %#v, want %#v", got, want)
	}
	if _, err := service.Consume(challenge.ID, challenge.Secret, now.Add(4*time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replayed Consume error = %v, want ErrNotFound", err)
	}
}

func TestExpiredChallengeCannotBeApprovedOrConsumed(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(time.Minute)

	challenge, err := service.Create(now)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := service.Approve(challenge.ApprovalToken, TelegramUser{ID: 42}, now.Add(time.Minute)); !errors.Is(err, ErrExpired) {
		t.Fatalf("Approve at expiry error = %v, want ErrExpired", err)
	}
	if _, err := service.Consume(challenge.ID, challenge.Secret, now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Consume after expired approval error = %v, want ErrNotFound", err)
	}
}

func TestApprovalTokenParsing(t *testing.T) {
	if token, ok := ApprovalToken(StartParameter("abc_123")); !ok || token != "abc_123" {
		t.Fatalf("ApprovalToken = %q, %v", token, ok)
	}
	if _, ok := ApprovalToken("ref_42"); ok {
		t.Fatal("referral parameter must not be accepted as web login")
	}
}
