package server

import (
	"strings"
	"testing"
)

func TestLoopbackAddressesAreAccepted(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:34115", "localhost:34115", "[::1]:34115", "127.0.0.2:8080"} {
		if err := CheckLoopback(addr); err != nil {
			t.Fatalf("CheckLoopback(%q) = %v, want it accepted", addr, err)
		}
	}
}

func TestEveryInterfaceIsRefused(t *testing.T) {
	err := CheckLoopback(":34115")

	if err == nil {
		t.Fatal("binding every interface was accepted; a LAN peer could drive the cluster with a forged Host header")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("err = %v, want it to say why", err)
	}
}

func TestARoutableAddressIsRefused(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:34115", "192.168.1.10:34115", "[::]:34115"} {
		if err := CheckLoopback(addr); err == nil {
			t.Fatalf("CheckLoopback(%q) accepted a non-loopback bind", addr)
		}
	}
}

func TestAMalformedAddressIsRefused(t *testing.T) {
	if err := CheckLoopback("34115"); err == nil {
		t.Fatal("a port with no host was accepted")
	}
	if err := CheckLoopback("not-an-address:x:y"); err == nil {
		t.Fatal("a malformed address was accepted")
	}
}

func TestAHostnameThatIsNotLocalhostIsRefused(t *testing.T) {
	if err := CheckLoopback("example.com:34115"); err == nil {
		t.Fatal("a routable hostname was accepted")
	}
}

func TestBackendPathsAreRecognised(t *testing.T) {
	for _, path := range []string{"/api/resources", "/api/exec", "/ws", "/healthz"} {
		if !IsBackendPath(path) {
			t.Fatalf("IsBackendPath(%q) = false; the desktop shell would serve it from the bundle", path)
		}
	}
}

func TestAssetPathsAreNotBackendPaths(t *testing.T) {
	for _, path := range []string{"/", "/index.html", "/assets/index-abc.js", "/apixyz"} {
		if IsBackendPath(path) {
			t.Fatalf("IsBackendPath(%q) = true; the desktop shell would proxy an asset", path)
		}
	}
}
