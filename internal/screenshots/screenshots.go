// Package screenshots names and places saved frames the way the frozen
// gridplayer does: a screenshot-template file name, a chosen folder, and a
// PNG or JPG output.
package screenshots

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Format is the image a saved frame is written as.
type Format uint8

const (
	PNG Format = iota
	JPG
)

// FormatFromName reads the frozen setting ("png" or "jpg").
func FormatFromName(name string) Format {
	if strings.EqualFold(strings.TrimSpace(name), "jpg") {
		return JPG
	}
	return PNG
}

// Name is the frozen setting value.
func (f Format) Name() string {
	if f == JPG {
		return "jpg"
	}
	return "png"
}

// Extension is the file extension the format writes.
func (f Format) Extension() string {
	if f == JPG {
		return "jpg"
	}
	return "png"
}

// The frozen limits and defaults.
const (
	DirName           = "screenshots"
	DefaultStem       = "screenshot"
	MaxStemLength     = 150
	MaxTitleLength    = 100
	MaxExtension      = 5
	DefaultTemplate   = "%F_%P"
	DefaultJPGQuality = 90
)

var unsafeChars = func() func(rune) bool {
	bad := `<>:"/\|?*`
	return func(r rune) bool {
		return strings.ContainsRune(bad, r) || r == 0 || (r < 0x20) || r == 0x7f
	}
}()

// timeLayouts maps the frozen strftime letters to Go's reference layout.
var timeLayouts = map[string]string{
	"Y": "2006", "y": "06", "m": "01", "d": "02",
	"H": "15", "M": "04", "S": "05",
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// Dir is the folder a screenshot is saved in: the setting when it is not
// blank, else the screenshots folder under the app data folder.
func Dir(setting, dataDir string) string {
	if trimmed := strings.TrimSpace(setting); trimmed != "" {
		if strings.HasPrefix(trimmed, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				trimmed = filepath.Join(home, strings.TrimPrefix(trimmed, "~"))
			}
		}
		return trimmed
	}
	return filepath.Join(dataDir, DirName)
}

// Position is a video position as it can go into a file name: 01-02-03.456.
func Position(positionMS int64, withMS bool) string {
	if positionMS < 0 {
		positionMS = 0
	}
	seconds := positionMS / 1000
	millis := positionMS % 1000
	text := fmt.Sprintf("%02d-%02d-%02d", seconds/3600, (seconds/60)%60, seconds%60)
	if !withMS {
		return text
	}
	return fmt.Sprintf("%s.%03d", text, millis)
}

// Unknown returns the template's specifiers that nothing fills in, as
// written.
func Unknown(template string) []string {
	var out []string
	for _, spec := range specifiers(template) {
		if !known(spec) {
			out = append(out, "%"+spec)
		}
	}
	return out
}

// HasCounter reports whether the template numbers its names.
func HasCounter(template string) bool {
	for _, spec := range specifiers(template) {
		if spec == "n" {
			return true
		}
	}
	return false
}

// Render fills a template for a frame. A specifier nothing fills in is left
// as it was written, as is a percent sign at the very end.
func Render(template, title string, positionMS int64, now time.Time, number int) string {
	title = truncate(title, MaxTitleLength)
	values := map[string]string{
		"f": title,
		"F": withoutExtension(title),
		"p": Position(positionMS, false),
		"P": Position(positionMS, true),
		"n": fmt.Sprintf("%04d", number),
		"D": now.Format("2006-01-02"),
		"T": now.Format("2006-01-02_15-04-05"),
		"%": "%",
	}
	var b strings.Builder
	for index := 0; index < len(template); {
		if template[index] != '%' {
			b.WriteByte(template[index])
			index++
			continue
		}
		spec, width := readSpecifier(template[index+1:])
		if spec == "" {
			// A percent sign at the very end stays one.
			b.WriteByte('%')
			index++
			continue
		}
		if !known(spec) {
			b.WriteString(template[index : index+1+width])
			index += 1 + width
			continue
		}
		if strings.HasPrefix(spec, "t") && len(spec) == 2 {
			b.WriteString(now.Format(timeLayouts[spec[1:]]))
		} else {
			b.WriteString(values[spec])
		}
		index += 1 + width
	}
	return Sanitize(b.String())
}

// Sanitize makes a name that can be a file name on every platform.
func Sanitize(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Map(func(r rune) rune {
		if unsafeChars(r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(truncate(name, MaxStemLength)), ". ")
	if name == "" {
		return DefaultStem
	}
	first := name
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		first = name[:dot]
	}
	if reservedNames[strings.ToUpper(first)] {
		name = "_" + name
	}
	return name
}

// NextPath is the file a new frame is written to. A template with %n counts
// from 0001; one without takes the first free " (N)" suffix.
func NextPath(dir, template, title string, positionMS int64, now time.Time, ext string) (string, error) {
	if dir == "" {
		return "", errors.New("screenshots: no folder")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if HasCounter(template) {
		for number := 1; number <= 100000; number++ {
			path := filepath.Join(dir, Render(template, title, positionMS, now, number)+"."+ext)
			if !exists(path) {
				return path, nil
			}
		}
		return "", errors.New("screenshots: no free number")
	}
	stem := Render(template, title, positionMS, now, 1)
	for number := 1; number <= 100000; number++ {
		suffix := ""
		if number > 1 {
			suffix = fmt.Sprintf(" (%d)", number)
		}
		path := filepath.Join(dir, stem+suffix+"."+ext)
		if !exists(path) {
			return path, nil
		}
	}
	return "", errors.New("screenshots: no free name")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// specifiers returns the template's specifiers in order, as written.
func specifiers(template string) []string {
	var out []string
	for index := 0; index < len(template); index++ {
		if template[index] != '%' {
			continue
		}
		spec, width := readSpecifier(template[index+1:])
		out = append(out, spec)
		index += width
	}
	return out
}

// readSpecifier reads the specifier after a '%', and how many further
// bytes it took; a percent sign at the very end is just a percent sign.
func readSpecifier(rest string) (string, int) {
	if rest == "" {
		return "", 0
	}
	if rest[0] == 't' {
		if len(rest) < 2 {
			return "t", 1
		}
		return rest[:2], 2
	}
	return rest[:1], 1
}

func known(spec string) bool {
	switch spec {
	case "f", "F", "p", "P", "n", "D", "T", "%":
		return true
	}
	return len(spec) == 2 && spec[0] == 't' && strings.ContainsRune("YymdHMS", rune(spec[1]))
}

// withoutExtension cuts what looks like a file extension, as a title is as
// often a URL as a name.
func withoutExtension(title string) string {
	dot := strings.LastIndexByte(title, '.')
	if dot <= 0 {
		return title
	}
	stem, ext := title[:dot], title[dot+1:]
	if ext == "" || len(ext) > MaxExtension {
		return title
	}
	for _, r := range ext {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return title
		}
	}
	return stem
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8Start(text[cut]) {
		cut--
	}
	return text[:cut]
}

func utf8Start(b byte) bool {
	return b&0xC0 != 0x80
}
