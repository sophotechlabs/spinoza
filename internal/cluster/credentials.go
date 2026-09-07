package cluster

import (
	"path/filepath"
	"strings"
)

const credentialMarker = "getting credentials: exec: executable "

const trustMarker = "tls: failed to verify certificate: "

var untrustedReasons = []string{"unknown authority", "is not trusted"}

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

func untrustedCertificate(err error) string {
	if err == nil {
		return ""
	}
	_, after, found := strings.Cut(err.Error(), trustMarker)
	if !found {
		return ""
	}
	reason, _, _ := strings.Cut(after, "\n")
	reason = strings.TrimSpace(reason)
	for _, known := range untrustedReasons {
		if strings.Contains(reason, known) {
			return reason
		}
	}
	return ""
}
