package gamelabels

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed pals.json
var palNamesJSON []byte

// palZH maps a Pal ID (original or lowercased) to its Chinese name. The source
// table also carries en/ja entries; only zh is used by the bot so the bot can
// return the same localized name as the web frontend instead of asking the AI
// to translate an English identifier.
var palZH = loadPalChineseNames()

func loadPalChineseNames() map[string]string {
	var data struct {
		ZH map[string]string `json:"zh"`
	}
	if err := json.Unmarshal(palNamesJSON, &data); err != nil {
		panic("parse embedded pal names: " + err.Error())
	}
	result := make(map[string]string, len(data.ZH)*2)
	for key, name := range data.ZH {
		if strings.TrimSpace(name) == "" {
			continue
		}
		result[key] = name
		result[strings.ToLower(key)] = name
	}
	return result
}

// PalChineseName returns the localized Chinese pal name, falling back to the
// save value for identifiers newer than the bundled table.
func PalChineseName(palID string) string {
	if palID == "" {
		return palID
	}
	if name, ok := palZH[palID]; ok {
		return name
	}
	if name, ok := palZH[strings.ToLower(palID)]; ok {
		return name
	}
	return palID
}
