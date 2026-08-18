package cluster

import (
	"path/filepath"
	"strings"
)

const credentialMarker = "getting credentials: exec: executable "

func credentialPlugin(err error) string {
	if err == nil {
		return ""
	}
	_, after, found := strings.Cut(err.Error(), credentialMarker)
	if !found {
		return ""
	}
	named, _, _ := strings.Cut(after, " ")
	if named == "" {
		return ""
	}
	return filepath.Base(named)
}
