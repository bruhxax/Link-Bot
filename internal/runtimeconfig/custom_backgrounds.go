package runtimeconfig

import (
	"errors"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var customBackgroundURL = regexp.MustCompile(`^/mini-app/uploads/background-[0-9a-f]{16}\.(png|jpg|gif|webp|avif|bmp|svg|mp4|webm|mov|ogv)$`)
var customBackgroundID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validateCustomBackgrounds(value *AppearanceSettings) error {
	if len(value.CustomBackgrounds) > 20 {
		return errors.New("можно сохранить не более 20 фонов")
	}
	ids := map[string]bool{}
	for i := range value.CustomBackgrounds {
		item := &value.CustomBackgrounds[i]
		if !customBackgroundID.MatchString(item.ID) || ids[item.ID] {
			return errors.New("некорректный или повторяющийся идентификатор фона")
		}
		ids[item.ID] = true
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = "Свой фон"
		}
		if utf8.RuneCountInString(item.Name) > 100 {
			return errors.New("название фона: не более 100 символов")
		}
		if !customBackgroundURL.MatchString(item.URL) {
			return errors.New("сначала загрузите фон файлом или по ссылке")
		}
		if item.Poster != "" && (!customBackgroundURL.MatchString(item.Poster) || path.Ext(item.Poster) != ".png") {
			return errors.New("некорректное превью фона")
		}
		ext := strings.TrimPrefix(path.Ext(item.URL), ".")
		expected := "image"
		switch ext {
		case "gif":
			expected = "gif"
		case "mp4", "webm", "mov", "ogv":
			expected = "video"
		}
		if item.Type != expected {
			return errors.New("формат фона не соответствует файлу")
		}
		if item.Fit == "" {
			item.Fit = "cover"
		}
		if item.Fit != "cover" && item.Fit != "contain" {
			return errors.New("некорректный режим размера фона")
		}
		if item.Scale < 50 || item.Scale > 300 || item.PositionX < 0 || item.PositionX > 100 || item.PositionY < 0 || item.PositionY > 100 || item.Speed < 10 || item.Speed > 200 || item.Dimming < 0 || item.Dimming > 90 {
			return errors.New("параметры фона вне допустимых границ")
		}
	}
	if value.ActiveBackground != "" && !ids[value.ActiveBackground] {
		return errors.New("выбранный фон отсутствует в списке")
	}
	if value.BackgroundMode == "custom" && len(ids) > 0 && !ids[value.ActiveBackground] {
		return errors.New("выберите свой фон")
	}
	return nil
}
