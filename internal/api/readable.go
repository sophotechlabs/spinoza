package api

import "encoding/json"

func ParseNamespaces(raw string) map[string][]string {
	if raw == "" {
		return nil
	}
	var held map[string][]string
	err := json.Unmarshal([]byte(raw), &held)
	if err != nil {
		return nil
	}
	return held
}
