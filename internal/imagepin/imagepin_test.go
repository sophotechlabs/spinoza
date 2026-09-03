package imagepin

import "testing"

func TestOnlyAFullSHA256DigestIsAPin(t *testing.T) {
	valid := "busybox@sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0"
	if !Valid(valid) {
		t.Fatal("a full sha256 digest was refused")
	}
	for _, image := range []string{
		"busybox:1.37",
		"busybox@sha256:9db7",
		"busybox@sha512:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0",
		"busybox@sha256:9DB7B59979C38555A39DEF84A31FB98B5296952F9E3AFD4F6F11F05B07ADFAB0",
	} {
		if Valid(image) {
			t.Fatalf("mutable or malformed image %q was accepted", image)
		}
	}
}
