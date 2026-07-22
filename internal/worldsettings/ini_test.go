package worldsettings

import (
	"strings"
	"testing"
)

func parseFixture(t *testing.T, content string) *Document {
	t.Helper()
	document, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return document
}

func TestOptionSettingsTokenizerHandlesQuotesListsEscapesAndUnknowns(t *testing.T) {
	content := sectionHeader + "\r\n" +
		`OptionSettings=(ServerName="中文,服务器\"A\"",AdminPassword="",CrossplayPlatforms=(Steam,Xbox,PS5,Mac),DenyTechnologyList=("GrapplingGun","GuildChest"),NestedUnknown=(A=(B,C),D="x,y"),UnknownKey="preserve=me")`
	document := parseFixture(t, content)
	if got, _ := document.Raw("ServerName"); got != `"中文,服务器\"A\""` {
		t.Fatalf("server name raw value = %q", got)
	}
	if got, _ := document.Raw("NestedUnknown"); got != `(A=(B,C),D="x,y")` {
		t.Fatalf("nested unknown = %q", got)
	}
	if got, _ := document.Raw("UnknownKey"); got != `"preserve=me"` {
		t.Fatalf("unknown value = %q", got)
	}

	definitions := SchemaByKey()
	platforms, err := DecodeValue(definitions["CrossplayPlatforms"], mustRaw(t, document, "CrossplayPlatforms"))
	if err != nil || len(platforms.([]string)) != 4 {
		t.Fatalf("decode platforms = %#v, %v", platforms, err)
	}
	technologies, err := DecodeValue(definitions["DenyTechnologyList"], mustRaw(t, document, "DenyTechnologyList"))
	if err != nil || len(technologies.([]string)) != 2 {
		t.Fatalf("decode technologies = %#v, %v", technologies, err)
	}
}

func mustRaw(t *testing.T, document *Document, key string) string {
	t.Helper()
	raw, ok := document.Raw(key)
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	return raw
}

func TestOptionSettingsRoundTripPreservesUnknownFieldsAndLineEnding(t *testing.T) {
	content := sectionHeader + "\r\n" + `OptionSettings=(UnknownFutureField=(A,B),ServerName="旧名称")`
	document := parseFixture(t, content)
	definition := SchemaByKey()["ServerName"]
	raw, err := EncodeValue(definition, "新名称,含逗号")
	if err != nil {
		t.Fatal(err)
	}
	if err := document.SetRaw("ServerName", raw); err != nil {
		t.Fatal(err)
	}
	written := string(document.Serialize())
	if !strings.Contains(written, "\r\n") {
		t.Fatal("CRLF prefix was not preserved")
	}
	if strings.HasSuffix(written, "\n") {
		t.Fatal("serializer added a final newline that the input did not have")
	}
	if !strings.Contains(written, "UnknownFutureField=(A,B)") {
		t.Fatal("unknown field was not preserved")
	}
	reparsed := parseFixture(t, written)
	decoded, err := DecodeValue(definition, mustRaw(t, reparsed, "ServerName"))
	if err != nil || decoded != "新名称,含逗号" {
		t.Fatalf("round-trip server name = %#v, %v", decoded, err)
	}
}

func TestOptionSettingsSupportsVeryLongSingleLine(t *testing.T) {
	longDescription := strings.Repeat("Palworld,", 20_000)
	raw, err := EncodeValue(SchemaByKey()["ServerDescription"], longDescription)
	if err != nil {
		t.Fatal(err)
	}
	document := parseFixture(t, sectionHeader+"\nOptionSettings=(ServerDescription="+raw+")")
	decoded, err := DecodeValue(SchemaByKey()["ServerDescription"], mustRaw(t, document, "ServerDescription"))
	if err != nil || decoded != longDescription {
		t.Fatalf("long OptionSettings round-trip failed: len=%d err=%v", len(decoded.(string)), err)
	}
}

func TestNormalizeValueRejectsRangeEnumAndListViolations(t *testing.T) {
	definitions := SchemaByKey()
	if _, err := NormalizeValue(definitions["BaseCampWorkerMaxNum"], float64(51)); err == nil {
		t.Fatal("worker maximum above official max was accepted")
	}
	if _, err := NormalizeValue(definitions["DeathPenalty"], "Everything"); err == nil {
		t.Fatal("unknown death penalty was accepted")
	}
	if _, err := NormalizeValue(definitions["CrossplayPlatforms"], []any{"Steam", "Unknown"}); err == nil {
		t.Fatal("unknown crossplay platform was accepted")
	}
}

func TestPasswordCanBeParsedAsEmptyWithoutBeingMasked(t *testing.T) {
	document := parseFixture(t, sectionHeader+`\nOptionSettings=(AdminPassword="",ServerPassword="secret")`)
	definitions := SchemaByKey()
	admin, err := DecodeValue(definitions["AdminPassword"], mustRaw(t, document, "AdminPassword"))
	if err != nil || admin != "" {
		t.Fatalf("empty password = %#v, %v", admin, err)
	}
	server, err := DecodeValue(definitions["ServerPassword"], mustRaw(t, document, "ServerPassword"))
	if err != nil || server != "secret" {
		t.Fatalf("password decode failed: %#v, %v", server, err)
	}
}
