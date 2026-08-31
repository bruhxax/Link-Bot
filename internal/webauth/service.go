package webauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	StartParameterPrefix = "web_login_"
	DefaultTTL           = 3 * time.Minute
	maxChallenges        = 10000
	randomTokenBytes     = 24
)

var (
	ErrCapacity       = errors.New("web login capacity reached")
	ErrNotFound       = errors.New("web login challenge not found")
	ErrPending        = errors.New("web login challenge is pending")
	ErrExpired        = errors.New("web login challenge expired")
	ErrInvalidSecret  = errors.New("invalid web login secret")
	ErrAlreadyClaimed = errors.New("web login challenge already claimed")
)

type TelegramUser struct {
	ID           int64
	FirstName    string
	LastName     string
	Username     string
	PhotoURL     string
	LanguageCode string
}

type Challenge struct {
	ID            string
	Secret        string
	ApprovalToken string
	ExpiresAt     time.Time
}

type challengeEntry struct {
	id            string
	secretHash    [sha256.Size]byte
	approvalToken string
	expiresAt     time.Time
	user          *TelegramUser
}

type Service struct {
	mu         sync.Mutex
	ttl        time.Duration
	byID       map[string]*challengeEntry
	byApproval map[string]string
}

func NewService(ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Service{
		ttl:        ttl,
		byID:       make(map[string]*challengeEntry),
		byApproval: make(map[string]string),
	}
}

func (s *Service) Create(now time.Time) (Challenge, error) {
	if s == nil {
		return Challenge{}, ErrCapacity
	}

	id, err := randomToken()
	if err != nil {
		return Challenge{}, err
	}
	secret, err := randomToken()
	if err != nil {
		return Challenge{}, err
	}
	approvalToken, err := randomToken()
	if err != nil {
		return Challenge{}, err
	}

	now = now.UTC()
	entry := &challengeEntry{
		id:            id,
		secretHash:    sha256.Sum256([]byte(secret)),
		approvalToken: approvalToken,
		expiresAt:     now.Add(s.ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	if len(s.byID) >= maxChallenges {
		return Challenge{}, ErrCapacity
	}
	s.byID[id] = entry
	s.byApproval[approvalToken] = id

	return Challenge{
		ID:            id,
		Secret:        secret,
		ApprovalToken: approvalToken,
		ExpiresAt:     entry.expiresAt,
	}, nil
}

func (s *Service) Approve(approvalToken string, user TelegramUser, now time.Time) error {
	if s == nil || strings.TrimSpace(approvalToken) == "" || user.ID <= 0 {
		return ErrNotFound
	}

	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byApproval[approvalToken]
	if !ok {
		return ErrNotFound
	}
	entry, ok := s.byID[id]
	if !ok {
		delete(s.byApproval, approvalToken)
		return ErrNotFound
	}
	if !now.Before(entry.expiresAt) {
		s.deleteLocked(entry)
		return ErrExpired
	}
	if entry.user != nil {
		return ErrAlreadyClaimed
	}

	approved := user
	entry.user = &approved
	return nil
}

func (s *Service) Consume(id, secret string, now time.Time) (TelegramUser, error) {
	if s == nil || strings.TrimSpace(id) == "" {
		return TelegramUser{}, ErrNotFound
	}

	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[id]
	if !ok {
		return TelegramUser{}, ErrNotFound
	}
	providedHash := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(providedHash[:], entry.secretHash[:]) != 1 {
		return TelegramUser{}, ErrInvalidSecret
	}
	if !now.Before(entry.expiresAt) {
		s.deleteLocked(entry)
		return TelegramUser{}, ErrExpired
	}
	if entry.user == nil {
		return TelegramUser{}, ErrPending
	}

	user := *entry.user
	s.deleteLocked(entry)
	return user, nil
}

func StartParameter(approvalToken string) string {
	return StartParameterPrefix + strings.TrimSpace(approvalToken)
}

func ApprovalToken(startParameter string) (string, bool) {
	if !strings.HasPrefix(startParameter, StartParameterPrefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(startParameter, StartParameterPrefix))
	return token, token != ""
}

func (s *Service) pruneLocked(now time.Time) {
	for _, entry := range s.byID {
		if !now.Before(entry.expiresAt) {
			s.deleteLocked(entry)
		}
	}
}

func (s *Service) deleteLocked(entry *challengeEntry) {
	delete(s.byID, entry.id)
	delete(s.byApproval, entry.approvalToken)
}

func randomToken() (string, error) {
	buffer := make([]byte, randomTokenBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
