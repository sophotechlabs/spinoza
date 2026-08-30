package api

import "encoding/json"

func ParseColumns(raw string) map[string][]CustomColumn {
	if raw == "" {
		return nil
	}
	var held map[string][]CustomColumn
	err := json.Unmarshal([]byte(raw), &held)
	if err != nil {
		return nil
	}
	return held
}
