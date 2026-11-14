package darwin

import (
	"bytes"
	"encoding/binary"
	"errors"
)

func parseProcArgs(data []byte) ([]string, error) {
	malformed := errors.New("进程参数数据无效")
	if len(data) < 5 {
		return nil, malformed
	}
	argc := int(binary.LittleEndian.Uint32(data[:4]))
	if argc < 1 || argc > 65536 {
		return nil, malformed
	}
	data = data[4:]
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return nil, malformed
	}
	data = data[end+1:]
	for len(data) > 0 && data[0] == 0 {
		data = data[1:]
	}
	args := make([]string, 0, argc)
	for i := 0; i < argc; i++ {
		end := bytes.IndexByte(data, 0)
		if end < 0 {
			return nil, malformed
		}
		args = append(args, string(data[:end]))
		data = data[end+1:]
	}
	return args, nil
}
