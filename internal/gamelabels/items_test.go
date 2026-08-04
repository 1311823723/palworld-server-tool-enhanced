package gamelabels

import "testing"

func TestItemChineseNameAndAliases(t *testing.T) {
	if got := ItemChineseName("stone", "Stone"); got != "石头" {
		t.Fatalf("stone Chinese name = %q", got)
	}
	for _, query := range []string{"石头", "Stone", "stone"} {
		if !MatchesItem("stone", "Stone", query) {
			t.Fatalf("stone did not match %q", query)
		}
	}
	if MatchesItem("wood", "Wood", "石头") {
		t.Fatal("wood unexpectedly matched 石头")
	}
}

func TestUnknownItemFallsBackToSaveName(t *testing.T) {
	if got := ItemChineseName("future_item", "Future Item"); got != "Future Item" {
		t.Fatalf("unknown item fallback = %q", got)
	}
}
