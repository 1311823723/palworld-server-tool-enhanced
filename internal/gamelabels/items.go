package gamelabels

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed items.json
var itemNamesJSON []byte

type itemNames struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	ZH  string `json:"zh"`
	EN  string `json:"en"`
}

var itemsByAlias = loadItemNames()

func loadItemNames() map[string]itemNames {
	var items []itemNames
	if err := json.Unmarshal(itemNamesJSON, &items); err != nil {
		panic("parse embedded item names: " + err.Error())
	}
	result := make(map[string]itemNames, len(items)*4)
	for _, item := range items {
		for _, alias := range []string{item.ID, item.Key, item.ZH, item.EN} {
			if normalized := normalize(alias); normalized != "" {
				result[normalized] = item
			}
		}
	}
	return result
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func lookup(itemID, fallback string) (itemNames, bool) {
	for _, value := range []string{itemID, fallback} {
		if item, ok := itemsByAlias[normalize(value)]; ok {
			return item, true
		}
	}
	return itemNames{}, false
}

// ItemChineseName returns the localized item name while preserving the save
// value as a fallback for identifiers that are newer than the bundled labels.
func ItemChineseName(itemID, fallback string) string {
	if item, ok := lookup(itemID, fallback); ok && strings.TrimSpace(item.ZH) != "" {
		return item.ZH
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return itemID
}

// MatchesItem checks the Chinese name, English name, item key and save ID.
// Location names are intentionally handled by the inventory service instead.
func MatchesItem(itemID, fallback, query string) bool {
	query = normalize(query)
	if query == "" {
		return true
	}
	values := []string{itemID, fallback}
	if item, ok := lookup(itemID, fallback); ok {
		values = append(values, item.ID, item.Key, item.ZH, item.EN)
	}
	for _, value := range values {
		if strings.Contains(normalize(value), query) {
			return true
		}
	}
	return false
}
