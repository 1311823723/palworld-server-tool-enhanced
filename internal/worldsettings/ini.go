package worldsettings

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const sectionHeader = "[/Script/Pal.PalGameWorldSettings]"

var ErrOptionSettingsNotFound = errors.New("PalWorldSettings.ini does not contain OptionSettings")

type Entry struct {
	Key      string
	RawValue string
}

type Document struct {
	before  string
	after   string
	entries []Entry
	index   map[string]int
}

func NewDocument() *Document {
	return &Document{
		before: sectionHeader + "\nOptionSettings=(",
		after:  ")\n",
		index:  make(map[string]int),
	}
}

func Parse(data []byte) (*Document, error) {
	text := string(data)
	if !strings.Contains(text, sectionHeader) {
		return nil, errors.New("PalWorldSettings.ini section header is missing")
	}
	marker := "OptionSettings="
	markerIndex := strings.Index(text, marker)
	if markerIndex < 0 {
		return nil, ErrOptionSettingsNotFound
	}
	openOffset := strings.Index(text[markerIndex+len(marker):], "(")
	if openOffset < 0 {
		return nil, errors.New("OptionSettings opening parenthesis is missing")
	}
	open := markerIndex + len(marker) + openOffset
	closeIndex, err := matchingParen(text, open)
	if err != nil {
		return nil, err
	}
	parts, err := splitTopLevel(text[open+1:closeIndex], ',')
	if err != nil {
		return nil, fmt.Errorf("tokenize OptionSettings: %w", err)
	}
	document := &Document{
		before:  text[:open+1],
		after:   text[closeIndex:],
		entries: make([]Entry, 0, len(parts)),
		index:   make(map[string]int, len(parts)),
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		key, raw, err := splitAssignment(part)
		if err != nil {
			return nil, err
		}
		if _, duplicate := document.index[key]; duplicate {
			return nil, fmt.Errorf("duplicate OptionSettings key %q", key)
		}
		document.index[key] = len(document.entries)
		document.entries = append(document.entries, Entry{Key: key, RawValue: strings.TrimSpace(raw)})
	}
	return document, nil
}

func matchingParen(text string, open int) (int, error) {
	depth := 0
	quoted := false
	escaped := false
	for index := open; index < len(text); index++ {
		char := text[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index, nil
			}
			if depth < 0 {
				return 0, errors.New("unexpected closing parenthesis")
			}
		}
	}
	if quoted {
		return 0, errors.New("unterminated quoted string")
	}
	return 0, errors.New("OptionSettings closing parenthesis is missing")
}

func splitTopLevel(value string, separator byte) ([]string, error) {
	parts := make([]string, 0)
	start, depth := 0, 0
	quoted, escaped := false, false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced nested parenthesis")
			}
		default:
			if char == separator && depth == 0 {
				parts = append(parts, value[start:index])
				start = index + 1
			}
		}
	}
	if quoted || depth != 0 {
		return nil, errors.New("unterminated quote or nested parenthesis")
	}
	parts = append(parts, value[start:])
	return parts, nil
}

func splitAssignment(value string) (string, string, error) {
	parts, err := splitTopLevel(value, '=')
	if err != nil {
		return "", "", err
	}
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid OptionSettings assignment %q", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" || strings.ContainsAny(key, "\r\n,()\"") {
		return "", "", fmt.Errorf("invalid OptionSettings key %q", key)
	}
	return key, strings.Join(parts[1:], "="), nil
}

func (document *Document) Serialize() []byte {
	entries := make([]string, 0, len(document.entries))
	for _, entry := range document.entries {
		entries = append(entries, entry.Key+"="+entry.RawValue)
	}
	return []byte(document.before + strings.Join(entries, ",") + document.after)
}

func (document *Document) Entries() []Entry {
	result := make([]Entry, len(document.entries))
	copy(result, document.entries)
	return result
}

func (document *Document) Raw(key string) (string, bool) {
	index, ok := document.index[key]
	if !ok {
		return "", false
	}
	return document.entries[index].RawValue, true
}

func (document *Document) SetRaw(key, rawValue string) error {
	if _, _, err := splitAssignment(key + "=" + rawValue); err != nil {
		return err
	}
	if _, err := splitTopLevel(rawValue, ','); err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}
	if index, ok := document.index[key]; ok {
		document.entries[index].RawValue = rawValue
		return nil
	}
	document.index[key] = len(document.entries)
	document.entries = append(document.entries, Entry{Key: key, RawValue: rawValue})
	return nil
}

func DecodeValue(definition Definition, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	switch definition.Type {
	case "boolean":
		if strings.EqualFold(raw, "true") {
			return true, nil
		}
		if strings.EqualFold(raw, "false") {
			return false, nil
		}
		return nil, errors.New("expected True or False")
	case "integer":
		value, err := strconv.ParseInt(raw, 10, 64)
		return value, err
	case "float":
		value, err := strconv.ParseFloat(raw, 64)
		return value, err
	case "string", "password":
		if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
			return strconv.Unquote(raw)
		}
		return raw, nil
	case "enum":
		return strings.Trim(raw, "\""), nil
	case "string_list", "technology_list", "platform_list":
		if !strings.HasPrefix(raw, "(") || !strings.HasSuffix(raw, ")") {
			if raw == "" {
				return []string{}, nil
			}
			return nil, errors.New("expected parenthesized list")
		}
		parts, err := splitTopLevel(raw[1:len(raw)-1], ',')
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) >= 2 && part[0] == '"' && part[len(part)-1] == '"' {
				unquoted, err := strconv.Unquote(part)
				if err != nil {
					return nil, err
				}
				part = unquoted
			}
			values = append(values, part)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported setting type %q", definition.Type)
	}
}

func EncodeValue(definition Definition, value any) (string, error) {
	normalized, err := NormalizeValue(definition, value)
	if err != nil {
		return "", err
	}
	switch definition.Type {
	case "boolean":
		if normalized.(bool) {
			return "True", nil
		}
		return "False", nil
	case "integer":
		return strconv.FormatInt(normalized.(int64), 10), nil
	case "float":
		return strconv.FormatFloat(normalized.(float64), 'f', 6, 64), nil
	case "string", "password":
		return strconv.Quote(normalized.(string)), nil
	case "enum":
		return normalized.(string), nil
	case "platform_list":
		return "(" + strings.Join(normalized.([]string), ",") + ")", nil
	case "string_list", "technology_list":
		values := normalized.([]string)
		quoted := make([]string, len(values))
		for i, item := range values {
			quoted[i] = strconv.Quote(item)
		}
		return "(" + strings.Join(quoted, ",") + ")", nil
	default:
		return "", fmt.Errorf("unsupported setting type %q", definition.Type)
	}
}

func NormalizeValue(definition Definition, value any) (any, error) {
	switch definition.Type {
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, errors.New("must be a boolean")
		}
		return boolean, nil
	case "integer":
		numberValue, ok := value.(float64)
		if !ok || numberValue != float64(int64(numberValue)) {
			if integer, integerOK := value.(int64); integerOK {
				numberValue, ok = float64(integer), true
			} else if integer, integerOK := value.(int); integerOK {
				numberValue, ok = float64(integer), true
			}
		}
		if !ok || numberValue != float64(int64(numberValue)) {
			return nil, errors.New("must be an integer")
		}
		if err := validateRange(definition, numberValue); err != nil {
			return nil, err
		}
		return int64(numberValue), nil
	case "float":
		numberValue, ok := value.(float64)
		if !ok {
			switch number := value.(type) {
			case int:
				numberValue, ok = float64(number), true
			case int64:
				numberValue, ok = float64(number), true
			}
		}
		if !ok {
			return nil, errors.New("must be a number")
		}
		if err := validateRange(definition, numberValue); err != nil {
			return nil, err
		}
		return numberValue, nil
	case "string", "password", "enum":
		stringValue, ok := value.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}
		if definition.Type == "enum" && len(definition.Options) > 0 && !contains(definition.Options, stringValue) {
			return nil, fmt.Errorf("must be one of %s", strings.Join(definition.Options, ", "))
		}
		return stringValue, nil
	case "string_list", "technology_list", "platform_list":
		var values []string
		switch list := value.(type) {
		case []string:
			values = append(values, list...)
		case []any:
			for _, item := range list {
				stringItem, ok := item.(string)
				if !ok {
					return nil, errors.New("list values must be strings")
				}
				values = append(values, stringItem)
			}
		default:
			return nil, errors.New("must be a list")
		}
		for _, item := range values {
			if strings.ContainsAny(item, "\r\n\x00") {
				return nil, errors.New("list values cannot contain control characters")
			}
			if definition.Type == "platform_list" && len(definition.Options) > 0 && !contains(definition.Options, item) {
				return nil, fmt.Errorf("unsupported platform %q", item)
			}
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported setting type %q", definition.Type)
	}
}

func validateRange(definition Definition, value float64) error {
	if definition.Minimum != nil && value < *definition.Minimum {
		return fmt.Errorf("must be at least %v", *definition.Minimum)
	}
	if definition.Maximum != nil && value > *definition.Maximum {
		return fmt.Errorf("must be at most %v", *definition.Maximum)
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
