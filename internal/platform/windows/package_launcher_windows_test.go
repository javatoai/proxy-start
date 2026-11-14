//go:build windows

package windows

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestPackagedLauncherReportsStartFailure(t *testing.T) {
	root := t.TempDir()
	resultPath := filepath.Join(root, "result.json")
	data, err := json.Marshal(packagedLaunchRequest{
		Launch:     packagedConfig(domain.LaunchSpec{Identity: domain.AppIdentity{Executable: filepath.Join(root, "missing.exe")}}),
		ResultPath: resultPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	handled, err := RunPackagedLaunch([]string{packagedLaunchFlag, base64.StdEncoding.EncodeToString(data)})
	if !handled || err != nil {
		t.Fatalf("start failure should produce a receipt: handled=%v err=%v", handled, err)
	}
	data, err = os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result packagedLaunchResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.ProcessID != 0 || !strings.Contains(result.Error, "missing.exe") {
		t.Fatalf("invalid failure receipt: %+v", result)
	}
	if _, err := os.Stat(resultPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary receipt was not cleaned up")
	}
}

func TestPackagedLauncherLeavesNormalAppArgumentsAlone(t *testing.T) {
	for _, args := range [][]string{nil, {"--check"}, {"--open-proxy-console"}} {
		if handled, err := RunPackagedLaunch(args); handled || err != nil {
			t.Fatalf("normal startup was intercepted: %v", args)
		}
	}
	if handled, err := RunPackagedLaunch([]string{packagedLaunchFlag, "invalid-base64"}); !handled || err == nil {
		t.Fatal("invalid internal request was not rejected")
	}
}
