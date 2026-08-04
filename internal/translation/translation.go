package translation

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	builtin "link-bot/translations"
)

type Translation map[string]string

type Manager struct {
	translations    map[string]Translation
	defaultLanguage string
	mu              sync.RWMutex
}

var (
	instance *Manager
	once     sync.Once
)

func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			translations:    make(map[string]Translation),
			defaultLanguage: "en",
		}
	})
	return instance
}

func (tm *Manager) InitTranslations(translationsDir string, defaultLanguage string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if defaultLanguage != "" {
		tm.defaultLanguage = defaultLanguage
	}

	tm.translations = make(map[string]Translation)
	if err := tm.loadDirectory(os.DirFS(translationsDir), ".", true); err != nil {
		slog.Warn("translations: external directory ignored", "error", err)
	}
	if err := tm.loadDirectory(builtin.Files, ".", false); err != nil {
		return fmt.Errorf("failed to load bundled translations: %w", err)
	}

	if len(tm.translations) == 0 {
		return fmt.Errorf("no translations found")
	}
	if _, exists := tm.translations[tm.defaultLanguage]; !exists {
		requested := tm.defaultLanguage
		tm.defaultLanguage = fallbackLanguage(tm.translations)
		slog.Warn("translations: default language unavailable, using fallback", "requested", requested, "fallback", tm.defaultLanguage)
	}

	return nil
}

func (tm *Manager) loadDirectory(source fs.FS, directory string, overwrite bool) error {
	files, err := fs.ReadDir(source, directory)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		langCode := strings.TrimSuffix(file.Name(), ".json")
		if _, exists := tm.translations[langCode]; exists && !overwrite {
			continue
		}
		content, err := fs.ReadFile(source, filepath.ToSlash(filepath.Join(directory, file.Name())))
		if err != nil {
			return fmt.Errorf("read %s: %w", file.Name(), err)
		}
		var value Translation
		if err := json.Unmarshal(content, &value); err != nil {
			return fmt.Errorf("parse %s: %w", file.Name(), err)
		}
		tm.translations[langCode] = value
	}
	return nil
}

func fallbackLanguage(values map[string]Translation) string {
	for _, candidate := range []string{"ru", "en"} {
		if _, exists := values[candidate]; exists {
			return candidate
		}
	}
	languages := make([]string, 0, len(values))
	for language := range values {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages[0]
}

func (tm *Manager) GetText(langCode, key string) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if translation, exists := tm.translations[langCode]; exists {
		if text, exists := translation[key]; exists && text != "" {
			return text
		}
	}

	if translation, exists := tm.translations[tm.defaultLanguage]; exists {
		if text, exists := translation[key]; exists {
			return text
		}
	}

	return key
}
