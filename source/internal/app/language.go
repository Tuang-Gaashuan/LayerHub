package app

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed locales/en.json
var englishCatalog []byte

var englishMessages = func() map[string]string {
	m := map[string]string{}
	if err := json.Unmarshal(englishCatalog, &m); err != nil {
		panic("invalid English catalog")
	}
	return m
}()

var localeToken = regexp.MustCompile(`__lh_expr_\d+__|%(?:\[\d+\])?[+#0 -]*\d*(?:\.\d+)?[sdqvf]`)

type translatedPattern struct {
	expression *regexp.Regexp
	target     string
	slots      []string
	weight     int
}

var languagePatterns []translatedPattern
var languagePatternsOnce sync.Once

func languageText(language, source string) string {
	if language != "en" {
		return source
	}
	key := strings.TrimSpace(source)
	if translated, ok := englishMessages[key]; ok {
		start := strings.Index(source, key)
		return source[:start] + translated + source[start+len(key):]
	}
	languagePatternsOnce.Do(func() {
		for key, target := range englishMessages {
			indices := localeToken.FindAllStringIndex(key, -1)
			if len(indices) == 0 || !strings.Contains(key, "%") {
				continue
			}
			var expression strings.Builder
			expression.WriteString("^")
			offset := 0
			slots := []string{}
			weight := len(key)
			for _, pair := range indices {
				expression.WriteString(regexp.QuoteMeta(key[offset:pair[0]]))
				expression.WriteString(`([\s\S]*?)`)
				slot := key[pair[0]:pair[1]]
				slots = append(slots, slot)
				weight -= len(slot)
				offset = pair[1]
			}
			expression.WriteString(regexp.QuoteMeta(key[offset:]))
			expression.WriteString("$")
			languagePatterns = append(languagePatterns, translatedPattern{regexp.MustCompile(expression.String()), target, slots, weight})
		}
		sort.Slice(languagePatterns, func(i, j int) bool { return languagePatterns[i].weight > languagePatterns[j].weight })
	})
	for _, p := range languagePatterns {
		match := p.expression.FindStringSubmatch(source)
		if match == nil {
			continue
		}
		values := map[string][]string{}
		for i, slot := range p.slots {
			values[slot] = append(values[slot], match[i+1])
		}
		return localeToken.ReplaceAllStringFunc(p.target, func(slot string) string {
			list := values[slot]
			if len(list) == 0 {
				return slot
			}
			values[slot] = list[1:]
			return list[0]
		})
	}
	return source
}

func requestLanguage(header string) string {
	best, quality := "zh-CN", -1.0
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(fields[0])
		language := ""
		if tag == "en" || strings.HasPrefix(tag, "en-") {
			language = "en"
		}
		if tag == "zh" || strings.HasPrefix(tag, "zh-") {
			language = "zh-CN"
		}
		if language == "" {
			continue
		}
		q := 1.0
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if strings.HasPrefix(field, "q=") {
				var err error
				q, err = strconv.ParseFloat(strings.TrimPrefix(field, "q="), 64)
				if err != nil || q < 0 || q > 1 {
					q = 0
				}
			}
		}
		if q > 0 && q > quality {
			best, quality = language, q
		}
	}
	return best
}

type languageReader interface{ QueryRow(string, ...any) *sql.Row }

func accountLanguage(q languageReader, uid int64) string {
	var language string
	if q.QueryRow("SELECT language FROM user_languages WHERE user_id=?", uid).Scan(&language) == nil && language == "en" {
		return "en"
	}
	return "zh-CN"
}
func (s *Service) initLanguage() error {
	_, err := s.DB.Exec("CREATE TABLE IF NOT EXISTS user_languages(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,language TEXT NOT NULL CHECK(language IN ('zh-CN','en')))")
	return err
}
func validLanguage(value string) bool { return value == "zh-CN" || value == "en" }
func saveLanguage(tx *sql.Tx, uid int64, language string) error {
	if !validLanguage(language) {
		return invalid("请选择简体中文或 English")
	}
	_, err := tx.Exec("INSERT INTO user_languages(user_id,language) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET language=excluded.language", uid, language)
	return err
}
func (s *Service) setLanguage(u User, language string) error {
	return s.transaction(func(tx *sql.Tx) error {
		if _, err := currentUser(tx, u.ID); err != nil {
			return err
		}
		return saveLanguage(tx, u.ID, language)
	})
}
func localizedFormat(language, source string, args ...any) string {
	return fmt.Sprintf(languageText(language, source), args...)
}
