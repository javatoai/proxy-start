package darwin

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// parsePlistXML parses the dictionary subset needed from an Info.plist while
// remaining independent of macOS commands. plutil is still used by the
// Darwin resolver to normalize binary plists before this parser runs.
func parsePlistXML(data []byte) (map[string]any, error) {
	return parseTopLevelDict(data)
}

func parseTopLevelDict(data []byte) (map[string]any, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return nil, errors.New("Info.plist 缺少顶层 dict")
			}
			return nil, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "dict" {
			return decodeDict(decoder)
		}
	}
}

func decodeDict(decoder *xml.Decoder) (map[string]any, error) {
	result := make(map[string]any)
	var key string
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch item := token.(type) {
		case xml.StartElement:
			if item.Name.Local == "key" {
				var text string
				if err := decoder.DecodeElement(&text, &item); err != nil {
					return nil, err
				}
				key = strings.TrimSpace(text)
				continue
			}
			value, err := decodeValue(decoder, item)
			if err != nil {
				return nil, err
			}
			if key != "" {
				result[key] = value
				key = ""
			}
		case xml.EndElement:
			if item.Name.Local == "dict" {
				return result, nil
			}
		}
	}
}

func decodeValue(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	if start.Name.Local == "dict" {
		return decodeDict(decoder)
	}
	if start.Name.Local == "array" {
		values := make([]any, 0)
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch item := token.(type) {
			case xml.StartElement:
				value, err := decodeValue(decoder, item)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			case xml.EndElement:
				if item.Name.Local == "array" {
					return values, nil
				}
			}
		}
	}
	var text string
	if err := decoder.DecodeElement(&text, &start); err != nil {
		return nil, err
	}
	if start.Name.Local == "true" {
		return true, nil
	}
	if start.Name.Local == "false" {
		return false, nil
	}
	return strings.TrimSpace(text), nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
