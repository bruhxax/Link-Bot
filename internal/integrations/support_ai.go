package integrations

// Support AI shares the existing encrypted credential store, but has its own
// admin UI and is never exposed through the public runtime configuration.
const ProviderSupportAI = "support_ai"

func (s *Service) SupportAISettings() (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.records[ProviderSupportAI]
	clone := make(map[string]string, len(rec.Config))
	for key, value := range rec.Config {
		clone[key] = value
	}
	return clone, rec.Enabled && configured(ProviderSupportAI, clone) && clone["model"] != ""
}

func init() {
	definitions = append(definitions, ProviderDefinition{
		ID: ProviderSupportAI, Name: "ИИ", Kind: "ai",
		Fields: []FieldDefinition{
			{Key: "apiUrl", Label: "API URL", Required: true},
			{Key: "apiKey", Label: "API ключ", Required: true, Secret: true},
			{Key: "model", Label: "Модель"},
			{Key: "prompt", Label: "Промпт"},
			{Key: "handoffAfter", Label: "Ответов ИИ до вызова человека"},
		},
	})
}
