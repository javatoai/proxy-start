package darwin

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestProcArgsStopsBeforeEnvironmentAndPreservesArgumentBoundaries(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 3)
	data = append(data, []byte("/Applications/Codex app/Codex\x00\x00Codex\x00--proxy-server=http://localhost:7897\x00\x00HTTP_PROXY=secret\x00")...)
	args, err := parseProcArgs(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"Codex", "--proxy-server=http://localhost:7897", ""}) {
		t.Fatalf("%#v", args)
	}
	for _, bad := range [][]byte{nil, {1, 0, 0, 0, 'x'}, data[:8]} {
		if _, err := parseProcArgs(bad); err == nil {
			t.Fatal("malformed arguments accepted")
		}
	}
}
