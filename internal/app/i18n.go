package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"hitbox-map/locales"
)

type languagePack struct {
	Locale   string            `json:"locale"`
	Name     string            `json:"name"`
	Messages map[string]string `json:"messages"`
}
type languageCatalog struct {
	Languages map[string]languagePack `json:"languages"`
	Warnings  []string                `json:"warnings"`
}

var localePattern = regexp.MustCompile(`^[a-zA-Z]{2,8}(?:-[a-zA-Z0-9]{1,8})*$`)
var placeholderPattern = regexp.MustCompile(`\{([0-9]+)\}`)
var builtInLanguages = readBuiltInLanguages()
var nativeCatalog = languageCatalog{Languages: builtInLanguages}
var nativeLocale = "en"

func readBuiltInLanguages() map[string]languagePack {
	result := map[string]languagePack{}
	for _, locale := range []string{"en", "zh", "ja"} {
		data, err := locales.Files.ReadFile(locale + ".json")
		var pack languagePack
		if err != nil || json.Unmarshal(data, &pack) != nil {
			panic("invalid embedded language: " + locale)
		}
		result[locale] = pack
	}
	return result
}

func localeDirectory() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(executable), "locales")
}

func placeholders(value string) string {
	items := placeholderPattern.FindAllString(value, -1)
	sort.Strings(items)
	// Repeating a placeholder is allowed; only its identity matters.
	unique := []string{}
	for _, item := range items {
		if len(unique) == 0 || unique[len(unique)-1] != item {
			unique = append(unique, item)
		}
	}
	return strings.Join(unique, ",")
}

// External packs are reloaded on page load. Missing or invalid entries keep
// built-in translations, or fall back to English for additional languages.
func loadLanguages(directory string) languageCatalog {
	catalog := languageCatalog{Languages: map[string]languagePack{}, Warnings: []string{}}
	for locale, original := range builtInLanguages {
		copy := original
		copy.Messages = map[string]string{}
		for key, value := range original.Messages {
			copy.Messages[key] = value
		}
		catalog.Languages[locale] = copy
	}
	if directory == "" {
		return catalog
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return catalog
	}
	if err != nil {
		catalog.Warnings = append(catalog.Warnings, "locales/")
		return catalog
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		func() {
			invalid := func() { catalog.Warnings = append(catalog.Warnings, entry.Name()) }
			file, err := os.Open(filepath.Join(directory, entry.Name()))
			if err != nil {
				invalid()
				return
			}
			defer file.Close()
			data, err := io.ReadAll(io.LimitReader(file, 262145))
			if err != nil || len(data) > 262144 {
				invalid()
				return
			}
			var pack languagePack
			if json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &pack) != nil || !localePattern.MatchString(pack.Locale) || strings.TrimSpace(pack.Name) == "" || len(pack.Name) > 160 || strings.ContainsRune(pack.Name, 0) || pack.Messages == nil {
				invalid()
				return
			}
			locale := strings.ToLower(pack.Locale)
			if !strings.EqualFold(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), locale) {
				invalid()
				return
			}
			if _, exists := catalog.Languages[locale]; !exists && len(catalog.Languages) >= 128 {
				invalid()
				return
			}
			merged, exists := catalog.Languages[locale]
			if !exists {
				merged = languagePack{Locale: locale, Name: pack.Name, Messages: map[string]string{}}
			}
			merged.Name = pack.Name
			badEntry := false
			for key, value := range pack.Messages {
				english, known := builtInLanguages["en"].Messages[key]
				if !known {
					continue
				}
				if strings.TrimSpace(value) == "" || len(value) > 8192 || strings.ContainsRune(value, 0) || placeholders(value) != placeholders(english) {
					badEntry = true
					continue
				}
				merged.Messages[key] = value
			}
			catalog.Languages[locale] = merged
			if badEntry {
				invalid()
			}
		}()
	}
	return catalog
}

func resolveLanguage(preferred []string, languages map[string]languagePack) string {
	for _, requested := range preferred {
		locale := strings.ToLower(strings.ReplaceAll(requested, "_", "-"))
		if !localePattern.MatchString(locale) {
			continue
		}
		for {
			if _, ok := languages[locale]; ok {
				return locale
			}
			at := strings.LastIndexByte(locale, '-')
			if at < 0 {
				break
			}
			locale = locale[:at]
		}
	}
	return "en"
}

func (catalog languageCatalog) text(locale, key string, args ...any) string {
	value := catalog.Languages[locale].Messages[key]
	if value == "" {
		value = catalog.Languages["en"].Messages[key]
	}
	if value == "" {
		value = builtInLanguages["en"].Messages[key]
	}
	if value == "" {
		return key
	}
	return placeholderPattern.ReplaceAllStringFunc(value, func(token string) string {
		index, _ := strconv.Atoi(token[1 : len(token)-1])
		if index >= len(args) {
			return token
		}
		if message, ok := args[index].(*translatedError); ok {
			return catalog.text(locale, message.Key, message.Args...)
		}
		return fmt.Sprint(args[index])
	})
}

func nativeText(key string, args ...any) string {
	return nativeCatalog.text(nativeLocale, key, args...)
}
func nativeError(err error) string {
	if message, ok := err.(*translatedError); ok {
		return nativeText(message.Key, message.Args...)
	}
	return err.Error()
}

type translatedError struct {
	Key  string
	Args []any
}

func messageError(key string, args ...any) error { return &translatedError{Key: key, Args: args} }
func (e *translatedError) Error() string {
	return (languageCatalog{Languages: builtInLanguages}).text("en", e.Key, e.Args...)
}
func (e *translatedError) Unwrap() error {
	for _, arg := range e.Args {
		if err, ok := arg.(error); ok {
			return err
		}
	}
	return nil
}
func errorMessage(err error) any {
	if message, ok := err.(*translatedError); ok {
		args := []any{}
		for _, value := range message.Args {
			if cause, ok := value.(error); ok {
				args = append(args, errorMessage(cause))
			} else {
				args = append(args, value)
			}
		}
		return object{"key": message.Key, "args": args}
	}
	return err.Error()
}
func errorResponse(err error) object {
	return object{"error": err.Error(), "errorMessage": errorMessage(err)}
}
