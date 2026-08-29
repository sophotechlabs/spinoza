package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/version"
)

func TestTheBinaryCarriesTheTermsItIsLicensedUnder(t *testing.T) {
	if !strings.Contains(licenseText, "FSL-1.1-ALv2") {
		t.Fatal("the embedded text does not name the license")
	}
	if !strings.Contains(licenseText, "WITHOUT WARRANTIES OF ANY KIND") {
		t.Fatal("the embedded text is missing the warranty disclaimer")
	}
	if !strings.HasSuffix(licenseText, "\n") {
		t.Fatal("printing the embedded text would not end the line")
	}
}

func TestTheLicenseFlagPrintsTheTermsAndStopsThere(t *testing.T) {
	var out bytes.Buffer

	if !printedNotice(&out, settings{showLicense: true}) {
		t.Fatal("the run went on instead of stopping at the license")
	}
	if out.String() != licenseText {
		t.Fatalf("printed %d bytes, want the whole license", out.Len())
	}
}

func TestTheVersionFlagPrintsTheVersionAndStopsThere(t *testing.T) {
	var out bytes.Buffer

	if !printedNotice(&out, settings{showVersion: true}) {
		t.Fatal("the run went on instead of stopping at the version")
	}
	if out.String() != version.String()+"\n" {
		t.Fatalf("printed %q", out.String())
	}
}

func TestAnOrdinaryStartPrintsNoNoticeAtAll(t *testing.T) {
	var out bytes.Buffer

	if printedNotice(&out, settings{}) {
		t.Fatal("an ordinary start was cut short by a notice")
	}
	if out.Len() != 0 {
		t.Fatalf("printed %q, want nothing", out.String())
	}
}
