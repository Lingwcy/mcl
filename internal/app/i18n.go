package app

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

const (
	LangZhHans = "zh-Hans"
	LangZhHant = "zh-Hant"
	LangEn     = "en"
	LangJa     = "ja"
)

// SupportedLanguages lists the launcher display languages in UI order.
var SupportedLanguages = []string{LangZhHans, LangZhHant, LangEn, LangJa}

// I18nSettings stores launcher UI language and resource localization preferences.
type I18nSettings struct {
	Language                string `json:"language,omitempty"`
	AutoSetInstanceLanguage bool   `json:"auto_set_instance_language,omitempty"`
	TranslateResources      bool   `json:"translate_resources,omitempty"`
	PrefixResourceFileName  bool   `json:"prefix_resource_file_name,omitempty"`
}

func DefaultI18nSettings() I18nSettings {
	return I18nSettings{
		AutoSetInstanceLanguage: true,
		TranslateResources:      true,
		PrefixResourceFileName:  true,
	}
}

func (i I18nSettings) Validate() error {
	if _, ok := NormalizeLanguage(i.Language); !ok {
		return errors.New("不支持的界面语言")
	}
	return nil
}

// NormalizeLanguage maps BCP 47 and POSIX locale tags into a supported launcher
// language tag, returning ("", true) when empty (auto-detect device language).
func NormalizeLanguage(tag string) (string, bool) {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return "", true
	}
	clean := trimmed
	if idx := strings.IndexByte(clean, '@'); idx >= 0 {
		clean = clean[:idx]
	}
	if idx := strings.IndexByte(clean, '.'); idx >= 0 {
		clean = clean[:idx]
	}
	clean = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(clean), "_", "-"))
	switch clean {
	case "", "c", "posix":
		return "", false
	}
	parts := strings.Split(clean, "-")
	switch parts[0] {
	case "en":
		return LangEn, true
	case "ja":
		return LangJa, true
	case "zh":
		for _, sub := range parts[1:] {
			switch sub {
			case "hant", "tw", "hk", "mo":
				return LangZhHant, true
			case "hans", "cn", "sg":
				return LangZhHans, true
			}
		}
		return LangZhHans, true
	default:
		return "", false
	}
}

// DetectDeviceLanguage infers the best-matching supported language from the
// supplied device preferred languages and POSIX locale environment variables.
func DetectDeviceLanguage(preferred []string) string {
	candidates := slices.Concat(preferred, []string{os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")})
	for _, candidate := range candidates {
		if code, ok := NormalizeLanguage(candidate); ok && code != "" {
			return code
		}
	}
	return LangZhHans
}

// EffectiveLanguage returns the configured launcher language or falls back to
// the inferred device language when unset.
func (i I18nSettings) EffectiveLanguage(preferred []string) string {
	if code, ok := NormalizeLanguage(i.Language); ok && code != "" {
		return code
	}
	return DetectDeviceLanguage(preferred)
}

// MinecraftLanguageCode maps the effective launcher language to Minecraft's options.txt lang code.
func (i I18nSettings) MinecraftLanguageCode(preferred []string) string {
	switch i.EffectiveLanguage(preferred) {
	case LangZhHant:
		return "zh_tw"
	case LangEn:
		return "en_us"
	case LangJa:
		return "ja_jp"
	default:
		return "zh_cn"
	}
}

// EnsureMinecraftOptionsLanguage sets lang and onboardAccessibility:false in
// <gameDir>/options.txt when absent, without overwriting existing user values.
func EnsureMinecraftOptionsLanguage(gameDir, mcLang string) error {
	if gameDir == "" || mcLang == "" {
		return nil
	}
	switch mcLang {
	case "zh_cn", "zh_tw", "en_us", "ja_jp":
	default:
		return errors.New("invalid minecraft language code")
	}
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return err
	}
	defer root.Close()

	const maxOptionsSize = 256 << 10
	info, err := root.Lstat("options.txt")
	if errors.Is(err, os.ErrNotExist) {
		f, openErr := root.OpenFile("options.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			return openErr
		}
		payload := fmt.Sprintf("lang:%s\nonboardAccessibility:false\n", mcLang)
		_, writeErr := io.WriteString(f, payload)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("options.txt is not a regular file")
	}
	if info.Size() > maxOptionsSize {
		return errors.New("options.txt exceeds size limit")
	}

	f, err := root.Open("options.txt")
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxOptionsSize+1))
	f.Close()
	if err != nil {
		return err
	}
	if len(data) > maxOptionsSize {
		return errors.New("options.txt exceeds size limit")
	}

	hasLang := false
	hasOnboard := false
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(rawLine, "lang:") {
			hasLang = true
		} else if strings.HasPrefix(rawLine, "onboardAccessibility:") {
			hasOnboard = true
		}
	}
	if hasLang && hasOnboard {
		return nil
	}

	var buf bytes.Buffer
	buf.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		buf.WriteByte('\n')
	}
	if !hasLang {
		fmt.Fprintf(&buf, "lang:%s\n", mcLang)
	}
	if !hasOnboard {
		buf.WriteString("onboardAccessibility:false\n")
	}

	tmpName := ".mcl-options-" + rand.Text() + ".tmp"
	tmp, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(buf.Bytes())
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = root.Remove(tmpName)
		return writeErr
	}
	if closeErr != nil {
		_ = root.Remove(tmpName)
		return closeErr
	}
	if err := root.Rename(tmpName, "options.txt"); err != nil {
		_ = root.Remove(tmpName)
		return err
	}
	return nil
}
