package darwin

import "testing"

func TestParsePlistXMLDictionaryAndArray(t *testing.T) {
	values, err := parsePlistXML([]byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleDisplayName</key><string>Codex Desktop</string><key>CFBundleIconFiles</key><array><string>AppIcon</string><string>Other</string></array><key>LSBackgroundOnly</key><false/></dict></plist>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := firstString(values, "CFBundleDisplayName"); got != "Codex Desktop" {
		t.Fatalf("display name = %q", got)
	}
	icons, ok := values["CFBundleIconFiles"].([]any)
	if !ok || len(icons) != 2 || icons[0] != "AppIcon" {
		t.Fatalf("icon files = %#v", values["CFBundleIconFiles"])
	}
	if enabled, ok := values["LSBackgroundOnly"].(bool); !ok || enabled {
		t.Fatalf("false value = %#v", values["LSBackgroundOnly"])
	}
}
