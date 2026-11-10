// Package i18n loads GridPlayer's Qt .ts translation files.
package i18n

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Language is one selectable translation.
type Language struct {
	Code string
	Name string
}

// Languages lists the frozen source's translations, English first.
var Languages = []Language{
	{"en_US", "English"},
	{"pl_PL", "Polski"},
	{"ar_SA", "العربية"},
	{"cs_CZ", "Čeština"},
	{"de_DE", "Deutsch"},
	{"es_ES", "Español"},
	{"fr_FR", "Français"},
	{"hu_HU", "Magyar"},
	{"it_IT", "Italiano"},
	{"ja_JP", "日本語"},
	{"ko_KR", "한국어"},
	{"nl_NL", "Nederlands"},
	{"no_NO", "Norsk"},
	{"pt_BR", "Português"},
	{"ru_RU", "Русский"},
	{"sv_SE", "Svenska"},
	{"vi_VN", "Tiếng Việt"},
	{"zh_CN", "简体中文"},
	{"zh_TW", "繁體中文"},
}

// Translator holds one language's messages.
type Translator struct {
	Language string
	messages map[string]map[string]string
	bySource map[string]string
}

type tsDocument struct {
	Language string      `xml:"language,attr"`
	Contexts []tsContext `xml:"context"`
}

type tsContext struct {
	Name     string      `xml:"name"`
	Messages []tsMessage `xml:"message"`
}

type tsMessage struct {
	Source      string `xml:"source"`
	Translation string `xml:"translation"`
}

// Load reads a language's .ts file from dir. English returns an empty
// translator, since the source strings are already English.
func Load(dir, code string) (*Translator, error) {
	code = normalizeCode(code)
	translator := &Translator{
		Language: code,
		messages: map[string]map[string]string{},
		bySource: map[string]string{},
	}
	if code == "en_US" {
		return translator, nil
	}
	path := filepath.Join(dir, code+".ts")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("i18n: %w", err)
	}
	var document tsDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("i18n: %s: %w", path, err)
	}
	for _, context := range document.Contexts {
		if context.Name == "" {
			continue
		}
		if translator.messages[context.Name] == nil {
			translator.messages[context.Name] = map[string]string{}
		}
		for _, message := range context.Messages {
			source := strings.TrimSpace(message.Source)
			translation := strings.TrimSpace(message.Translation)
			if source == "" || translation == "" {
				continue
			}
			translator.messages[context.Name][source] = translation
			if _, exists := translator.bySource[source]; !exists {
				translator.bySource[source] = translation
			}
		}
	}
	return translator, nil
}

// Tr translates source in context, falling back to the source string.
func (t *Translator) Tr(context, source string) string {
	if t == nil {
		return source
	}
	if messages := t.messages[context]; messages != nil {
		if translation := messages[source]; translation != "" {
			return translation
		}
	}
	if translation := t.bySource[source]; translation != "" {
		return translation
	}
	return source
}

// Name returns the native name for a language code.
func Name(code string) string {
	code = normalizeCode(code)
	for _, language := range Languages {
		if language.Code == code {
			return language.Name
		}
	}
	return "English"
}

// Code returns the language code for a native name.
func Code(name string) string {
	for _, language := range Languages {
		if language.Name == name {
			return language.Code
		}
	}
	return "en_US"
}

func normalizeCode(code string) string {
	code = strings.TrimSpace(strings.ReplaceAll(code, "-", "_"))
	for _, language := range Languages {
		if strings.EqualFold(language.Code, code) {
			return language.Code
		}
	}
	return "en_US"
}

// ErrUnsupported reports a language with no bundled translation.
var ErrUnsupported = errors.New("i18n: unsupported language")
