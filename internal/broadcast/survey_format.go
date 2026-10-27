package broadcast

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"
	xhtml "golang.org/x/net/html"
)

var surveyHTMLPattern = regexp.MustCompile(`(?i)</?(?:b|strong|i|em|u|ins|s|strike|del|a|code|pre|blockquote|tg-spoiler|tg-emoji|span)(?:\s|/?>)`)

func validateSurveyMessage(text string, formatted bool) error {
	visible := text
	if formatted || looksLikeBroadcastHTML(text) {
		if err := ValidateHTML(text, formatted); err != nil {
			return fmt.Errorf("%w: %v", ErrSurvey, err)
		}
		tokenizer := xhtml.NewTokenizer(strings.NewReader(text))
		var out strings.Builder
		for {
			kind := tokenizer.Next()
			if kind == xhtml.ErrorToken {
				break
			}
			if kind == xhtml.TextToken {
				out.Write(tokenizer.Text())
			}
		}
		visible = out.String()
	}
	n := len(utf16.Encode([]rune(visible)))
	if strings.TrimSpace(visible) == "" || n > 3500 {
		return fmt.Errorf("%w: текст сообщения от 1 до 3500 символов", ErrSurvey)
	}
	return nil
}

// Telegram entity offsets use UTF-16, not byte or rune indices. Keep complete
// Unicode boundaries and nested ranges so emoji never shift adjacent styling.
func surveyMessageHTML(text string, entities []models.MessageEntity) (string, error) {
	if len(entities) == 0 && surveyHTMLPattern.MatchString(text) {
		if err := validateSurveyMessage(text, false); err != nil {
			return "", err
		}
		return text, nil
	}
	boundaries := map[int]int{0: 0}
	units := 0
	for offset, r := range text {
		boundaries[units] = offset
		units += utf16.RuneLen(r)
	}
	boundaries[units] = len(text)
	type span struct {
		start, end  int
		open, close string
	}
	spans := make([]span, 0, len(entities))
	for _, entity := range entities {
		start, ok := boundaries[entity.Offset]
		end, endOK := boundaries[entity.Offset+entity.Length]
		if !ok || !endOK || entity.Length <= 0 {
			return "", fmt.Errorf("%w: неверные границы форматирования", ErrSurvey)
		}
		opening, closing := "", ""
		tag := func(name string) { opening = "<" + name + ">"; closing = "</" + name + ">" }
		switch entity.Type {
		case models.MessageEntityTypeBold:
			tag("b")
		case models.MessageEntityTypeItalic:
			tag("i")
		case models.MessageEntityTypeUnderline:
			tag("u")
		case models.MessageEntityTypeStrikethrough:
			tag("s")
		case models.MessageEntityTypeSpoiler:
			tag("tg-spoiler")
		case models.MessageEntityTypeCode:
			tag("code")
		case models.MessageEntityTypeBlockquote:
			tag("blockquote")
		case models.MessageEntityTypeExpandableBlockquote:
			opening = "<blockquote expandable>"
			closing = "</blockquote>"
		case models.MessageEntityTypePre:
			tag("pre")
			if entity.Language != "" {
				opening = "<pre><code class=\"language-" + html.EscapeString(entity.Language) + "\">"
				closing = "</code></pre>"
			}
		case models.MessageEntityTypeCustomEmoji:
			if !customEmojiIDPattern.MatchString(entity.CustomEmojiID) {
				return "", fmt.Errorf("%w: неверный premium emoji", ErrSurvey)
			}
			opening = "<tg-emoji emoji-id=\"" + entity.CustomEmojiID + "\">"
			closing = "</tg-emoji>"
		case models.MessageEntityTypeTextLink, models.MessageEntityTypeTextMention, models.MessageEntityTypeURL, models.MessageEntityTypeEmail:
			target := entity.URL
			if entity.Type == models.MessageEntityTypeURL {
				target = text[start:end]
				if !strings.Contains(target, ":") {
					target = "https://" + target
				}
			}
			if entity.Type == models.MessageEntityTypeEmail {
				target = "mailto:" + text[start:end]
			}
			if entity.Type == models.MessageEntityTypeTextMention {
				if entity.User == nil || entity.User.ID <= 0 {
					return "", fmt.Errorf("%w: неверное упоминание", ErrSurvey)
				}
				target = "tg://user?id=" + strconv.FormatInt(entity.User.ID, 10)
			}
			parsed, err := url.Parse(target)
			if err != nil || !map[string]bool{"http": true, "https": true, "tg": true, "mailto": true}[strings.ToLower(parsed.Scheme)] {
				return "", fmt.Errorf("%w: неподдерживаемая ссылка", ErrSurvey)
			}
			opening = "<a href=\"" + html.EscapeString(target) + "\">"
			closing = "</a>"
		default:
			continue // Automatic mentions, commands and phone numbers retain their text.
		}
		spans = append(spans, span{start, end, opening, closing})
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})
	var result strings.Builder
	stack := []span{}
	position, index := 0, 0
	for position < len(text) || index < len(spans) || len(stack) > 0 {
		for len(stack) > 0 && stack[len(stack)-1].end == position {
			result.WriteString(stack[len(stack)-1].close)
			stack = stack[:len(stack)-1]
		}
		for index < len(spans) && spans[index].start == position {
			current := spans[index]
			if len(stack) > 0 && current.end > stack[len(stack)-1].end {
				return "", fmt.Errorf("%w: пересекающиеся границы форматирования", ErrSurvey)
			}
			result.WriteString(current.open)
			stack = append(stack, current)
			index++
		}
		next := len(text)
		if index < len(spans) && spans[index].start < next {
			next = spans[index].start
		}
		if len(stack) > 0 && stack[len(stack)-1].end < next {
			next = stack[len(stack)-1].end
		}
		if next > position {
			result.WriteString(html.EscapeString(text[position:next]))
			position = next
		} else if position == len(text) && len(stack) == 0 {
			break
		} else {
			return "", fmt.Errorf("%w: некорректное форматирование", ErrSurvey)
		}
	}
	value := result.String()
	if err := validateSurveyMessage(value, true); err != nil {
		return "", err
	}
	return value, nil
}
