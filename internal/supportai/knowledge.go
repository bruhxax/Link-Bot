package supportai

import (
	"regexp"
	"strings"
	"unicode"
)

// Product facts, not guessed UI/account workflows. Configured FAQ takes
// precedence when describing this particular service's rules.
const BasicKnowledge = `Подписка VPN-сервиса и импорт подписки в приложение — разные действия. Фраза «подписка не добавляется в Happ на ПК» означает проблему импорта; не переводить разговор на оплату без упоминания оплаты пользователем.
В кабинете есть «Подключиться» и экран настройки: пользователь выбирает устройство и клиент, открывает или копирует ссылку своей выбранной подписки. Подписку импортируют на каждом устройстве. В проекте нет подтверждения, что вход в аккаунт Happ синхронизирует подписки: не советовать такой вход.
Happ принимает подписку через буфер обмена, QR-код или диплинк. Общая инструкция: на нужном устройстве открыть кабинет → «Подключиться» → выбрать Windows/macOS/другую платформу и установленный клиент → импортировать доступ. Если автоматическое открытие не сработало, скопировать ссылку в кабинете и добавить её через буфер обмена в клиенте. Точные названия неизвестных кнопок приложения не выдумывать. Источник: https://www.happ.su/main/ru/faq/adding-configuration-subscription
Если импорт не проходит, нужен точный текст ошибки; не просить полную секретную ссылку подписки. Работает на телефоне, но не на ПК — сначала проверять импорт и клиент ПК, а не предлагать заново оплатить или утверждать, что подписка неактивна.
Если серверы не отображаются или показывают n/a, обновить импортированную подписку и проверить соединение; не обещать, что это обязательно исправит проблему. После импорта выбрать сервер и включить подключение. Для установки клиента использовать экран настройки сервиса.
Если подписка истекла, продление доступно пользователю через «Тарифы». ИИ ничего не начисляет и не изменяет. После оплаты ориентироваться на фактический статус платежа и подписки из контекста; несоответствие передавать администратору. Лимит устройств и трафика брать из аккаунта, не придумывать.
Проблемы со всеми серверами проверять через «Статус серверов». Если ошибка неясна, задать ровно один уточняющий вопрос, затем предложить короткие шаги. Не советовать переустановку или удаление данных без причины.`

var privatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:https?://|(?:vless|vmess|trojan|ss|ssr|happ|incy)://)[^\s<>"']+`),
	regexp.MustCompile(`(?i)\b(?:sk[_-]|pk[_-]|eyJ)[a-z0-9_.-]{8,}\b`),
	regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b|@[a-z][a-z0-9_]{3,}`),
	regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b|(?i)\b[0-9a-f]{8}-[0-9a-f-]{27,}\b`),
	regexp.MustCompile(`\+?[0-9][0-9 ()-]{8,}[0-9]|\b[0-9]{5,}\b`),
	regexp.MustCompile(`(?i)(?:пароль|password|api[_ -]?key|token|токен)\s*[:=]\s*\S+`),
}

func Redact(text string, identities ...string) string {
	for _, identity := range identities {
		if strings.TrimSpace(identity) != "" {
			text = regexp.MustCompile("(?i)"+regexp.QuoteMeta(identity)).ReplaceAllString(text, "[данные скрыты]")
		}
	}
	for _, pattern := range privatePatterns {
		text = pattern.ReplaceAllString(text, "[данные скрыты]")
	}
	return text
}

func LimitText(text string, size int) string {
	runes := []rune(text)
	if len(runes) > size {
		return string(runes[:size]) + "…"
	}
	return text
}

func SearchQuery(text string) string {
	text = strings.ToLower(Redact(text))
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	stop := " здравствуйте привет спасибо пожалуйста просто меня тебе почему какой какая какие что это как для при или есть нет было будет работает помогите подписка подписку подписки данные скрыты речь "
	seen := map[string]bool{}
	terms := []string{}
	for _, word := range words {
		if (len([]rune(word)) < 3 && word != "пк") || strings.Contains(stop, " "+word+" ") || seen[word] {
			continue
		}
		seen[word] = true
		terms = append(terms, word)
		if len(terms) >= 14 {
			break
		}
	}
	for _, word := range []string{"happ", "хапп", "нарр"} {
		if seen[word] {
			terms = append(terms, "happ", "хапп", "нарр")
			break
		}
	}
	if seen["пк"] || seen["windows"] {
		terms = append(terms, "windows", "компьютер")
	}
	return strings.Join(terms, " OR ")
}

var introductorySentence = regexp.MustCompile(`(?i)^\s*(?:а[, ]+)?(?:понял[а]?|понимаю|ясно|вы имеете в виду|речь (?:идёт|идет|о)).{0,240}?[.!?]\s*`)
var shortAcknowledgement = regexp.MustCompile(`(?i)^\s*(?:а[, ]+)?(?:понял[а]?|понимаю|ясно)[,:!\. ]+`)

func CleanReply(text string) string {
	text = strings.TrimSpace(text)
	if cleaned := introductorySentence.ReplaceAllString(text, ""); strings.TrimSpace(cleaned) != "" {
		text = cleaned
	}
	text = shortAcknowledgement.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}
