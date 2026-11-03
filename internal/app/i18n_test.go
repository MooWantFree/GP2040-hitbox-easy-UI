package app

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltInLanguagesComplete(t *testing.T) {
	english := builtInLanguages["en"].Messages
	for _, locale := range []string{"en", "zh", "ja"} {
		pack := builtInLanguages[locale]
		if pack.Locale != locale || pack.Name == "" || len(pack.Messages) != len(english) {
			t.Fatalf("incomplete language: %s", locale)
		}
		for key, source := range english {
			translation := pack.Messages[key]
			if strings.TrimSpace(translation) == "" || placeholders(source) != placeholders(translation) {
				t.Errorf("missing text or wrong placeholders: %s %s", locale, key)
			}
		}
	}
}

func TestResolveLanguage(t *testing.T) {
	for _, tc := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"ja-JP", "en-US"}, "ja"},
		{[]string{"zh-Hant-TW"}, "zh"},
		{[]string{"zh_CN"}, "zh"},
		{[]string{"de-DE", "ja-JP"}, "ja"},
		{[]string{"en-GB", "zh-CN"}, "en"},
		{[]string{"fr-FR"}, "en"},
		{nil, "en"},
	} {
		if got := resolveLanguage(tc.preferred, builtInLanguages); got != tc.want {
			t.Errorf("%v: got %s, want %s", tc.preferred, got, tc.want)
		}
	}
}

func writeLanguage(t *testing.T, directory, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, filename), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCustomLanguagesReloadAndFallback(t *testing.T) {
	directory := t.TempDir()
	writeLanguage(t, directory, "fr.json", "\ufeff"+`{"locale":"fr","name":"Français","messages":{"page.title":"Boutons","layout.bound":"Identifiés : {0} / {1}"}}`)
	writeLanguage(t, directory, "pt-BR.json", `{"locale":"pt-BR","name":"Português (Brasil)","messages":{"page.title":"Botões"}}`)
	writeLanguage(t, directory, "zh.json", `{"locale":"zh","name":"中文","messages":{"page.title":"自定义标题"}}`)
	catalog := loadLanguages(directory)
	if len(catalog.Warnings) != 0 || len(catalog.Languages) != 5 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	if resolveLanguage([]string{"pt-BR"}, catalog.Languages) != "pt-br" {
		t.Fatal("regional locale did not match")
	}
	if got := catalog.text("fr", "layout.bound", 3, 16); got != "Identifiés : 3 / 16" {
		t.Fatal(got)
	}
	if got := catalog.text("fr", "connection.connect"); got != "Connect" {
		t.Fatal(got)
	}
	if catalog.text("zh", "page.title") != "自定义标题" || catalog.text("zh", "connection.connect") != builtInLanguages["zh"].Messages["connection.connect"] {
		t.Fatal("partial override failed")
	}
	if builtInLanguages["zh"].Messages["page.title"] == "自定义标题" {
		t.Fatal("embedded catalog mutated")
	}
	a := newApp(t.TempDir(), nil)
	a.localeDir = directory
	read := func() languageCatalog {
		t.Helper()
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:29301/api/locales", nil))
		var got languageCatalog
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("language endpoint: %d %s", w.Code, w.Body.String())
		}
		return got
	}
	if read().text("fr", "page.title") != "Boutons" {
		t.Fatal("custom pack not served")
	}
	writeLanguage(t, directory, "fr.json", `{"locale":"fr","name":"Français","messages":{"page.title":"Touches"}}`)
	if read().text("fr", "page.title") != "Touches" {
		t.Fatal("pack not reloaded")
	}
	if err := os.Remove(filepath.Join(directory, "fr.json")); err != nil {
		t.Fatal(err)
	}
	if _, exists := read().Languages["fr"]; exists {
		t.Fatal("deleted pack still served")
	}
}

func TestInvalidLanguagesPreserveDefaults(t *testing.T) {
	directory := t.TempDir()
	for filename, content := range map[string]string{
		"broken.json": `{`,
		"de.json":     `{"locale":"fr","name":"Mismatch","messages":{}}`,
		"ja.json":     `{"locale":"ja","name":"日本語","messages":{"layout.bound":"{0}","common.cancel":"","tray.exit":"bad\u0000value"}}`,
		"fr.json":     `{"locale":"fr","name":"Français","messages":{"layout.bound":"{0}","common.close":"<b>Fermer</b>","unknown.key":"ignored"}}`,
		"es.json":     `{"locale":"es","name":"Español","messages":{"common.close":42}}`,
		"it.json":     strings.Repeat(" ", 262145),
	} {
		writeLanguage(t, directory, filename, content)
	}
	catalog := loadLanguages(directory)
	if len(catalog.Warnings) != 6 {
		t.Fatalf("warnings: %v", catalog.Warnings)
	}
	if catalog.text("ja", "layout.bound", 2, 16) != (languageCatalog{Languages: builtInLanguages}).text("ja", "layout.bound", 2, 16) {
		t.Fatal("bad placeholders replaced default")
	}
	if catalog.text("fr", "layout.bound", 2, 16) != "2 / 16 identified" {
		t.Fatal("missing translation did not fall back to English")
	}
	if catalog.text("fr", "common.close") != "<b>Fermer</b>" {
		t.Fatal("plain text translation altered")
	}
	if _, exists := catalog.Languages["fr"].Messages["unknown.key"]; exists {
		t.Fatal("unknown key retained")
	}
	if len(loadLanguages(filepath.Join(directory, "missing")).Warnings) != 0 {
		t.Fatal("missing optional directory reported as error")
	}
}

func TestStructuredTranslatedErrors(t *testing.T) {
	cause := errors.New("disk full")
	err := messageError("error.backupAborted", messageError("error.layoutSave", cause))
	if !errors.Is(err, cause) {
		t.Fatal("lost wrapped error")
	}
	response := errorResponse(err)
	want := object{"key": "error.backupAborted", "args": []any{object{"key": "error.layoutSave", "args": []any{"disk full"}}}}
	if !reflect.DeepEqual(response["errorMessage"], want) {
		t.Fatalf("lost structured cause: %v", response)
	}
	for _, locale := range []string{"en", "zh", "ja"} {
		catalog := languageCatalog{Languages: builtInLanguages}
		got := catalog.text(locale, "error.backupAborted", messageError("error.layoutSave", cause))
		want := catalog.text(locale, "error.backupAborted", catalog.text(locale, "error.layoutSave", "disk full"))
		if got != want || strings.Contains(got, "{0}") {
			t.Errorf("nested error in %s: %s", locale, got)
		}
	}
}
