//go:build windows

package windows

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRunPowerShellJSONUTF8RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := runPowerShellJSON(ctx, `[pscustomobject]@{Name='中文软件 · 代理'} | ConvertTo-Json -Compress`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal([]byte(output), &row); err != nil {
		t.Fatalf("decode UTF-8 JSON %q: %v", output, err)
	}
	if row.Name != "中文软件 · 代理" {
		t.Fatalf("PowerShell text was not preserved: %q", row.Name)
	}
}
