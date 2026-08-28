#!/usr/bin/env sh
set -eu

repo="sophotechlabs/spinoza"
download="https://github.com/$repo/releases/download"
out="${OUT_DIR:-dist/packaging}"
list="${CHECKSUMS:-dist/release/checksums.txt}"

version="${SPINOZA_VERSION:-}"
if [ -z "$version" ]; then
    die "SPINOZA_VERSION is required"
fi
numeric="${version#v}"

die() {
    echo "render: $1" >&2
    exit 1
}

hash_of() {
    found="$(awk -v name="$1" '$2 == name { print $1 }' "$list")"
    if [ -z "$found" ]; then
        die "$1 is not listed in $list"
    fi
    printf '%s' "$found"
}

asset() {
    printf 'spinoza_%s_%s_%s.%s' "$version" "$1" "$2" "$3"
}

url_of() {
    printf '%s/%s/%s' "$download" "$version" "$1"
}

if [ ! -f "$list" ]; then
    die "$list is missing, run just release-dist first"
fi

mkdir -p "$out/scoop" "$out/homebrew" "$out/krew" "$out/winget" "$out/chocolatey/tools"

windows_amd64="$(asset windows amd64 zip)"
windows_arm64="$(asset windows arm64 zip)"
darwin_amd64="$(asset darwin amd64 tar.gz)"
darwin_arm64="$(asset darwin arm64 tar.gz)"
linux_amd64="$(asset linux amd64 tar.gz)"
linux_arm64="$(asset linux arm64 tar.gz)"

cat > "$out/scoop/spinoza.json" <<JSON
{
  "version": "$numeric",
  "description": "Self-hosted Kubernetes GUI: one binary, browser tab or desktop window",
  "homepage": "https://spinoza.tech",
  "license": {
    "identifier": "FSL-1.1-ALv2",
    "url": "https://github.com/$repo/blob/main/LICENSE"
  },
  "architecture": {
    "64bit": {
      "url": "$(url_of "$windows_amd64")",
      "hash": "$(hash_of "$windows_amd64")"
    },
    "arm64": {
      "url": "$(url_of "$windows_arm64")",
      "hash": "$(hash_of "$windows_arm64")"
    }
  },
  "bin": "spinoza.exe",
  "checkver": {
    "github": "https://github.com/$repo"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "$download/v\$version/spinoza_v\$version_windows_amd64.zip"
      },
      "arm64": {
        "url": "$download/v\$version/spinoza_v\$version_windows_arm64.zip"
      }
    },
    "hash": {
      "url": "$download/v\$version/checksums.txt"
    }
  }
}
JSON

cat > "$out/homebrew/spinoza.rb" <<RUBY
class Spinoza < Formula
  desc "Self-hosted Kubernetes GUI: one binary, browser tab or desktop window"
  homepage "https://spinoza.tech"
  version "$numeric"
  license :cannot_represent

  on_macos do
    on_arm do
      url "$(url_of "$darwin_arm64")"
      sha256 "$(hash_of "$darwin_arm64")"
    end
    on_intel do
      url "$(url_of "$darwin_amd64")"
      sha256 "$(hash_of "$darwin_amd64")"
    end
  end

  on_linux do
    on_arm do
      url "$(url_of "$linux_arm64")"
      sha256 "$(hash_of "$linux_arm64")"
    end
    on_intel do
      url "$(url_of "$linux_amd64")"
      sha256 "$(hash_of "$linux_amd64")"
    end
  end

  def install
    bin.install "spinoza"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/spinoza --version")
  end
end
RUBY

krew_platform() {
    cat <<YAML
    - selector:
        matchLabels:
          os: $1
          arch: $2
      uri: $(url_of "$3")
      sha256: $(hash_of "$3")
      bin: $4
      files:
        - from: $4
          to: .
        - from: LICENSE
          to: .
YAML
}

{
    cat <<YAML
apiVersion: krew.googlecontainertools.github.com/v1alpha2
kind: Plugin
metadata:
  name: spinoza
spec:
  version: $version
  homepage: https://spinoza.tech
  shortDescription: Kubernetes GUI served on localhost
  description: |
    Spinoza serves a Kubernetes GUI from a single binary on localhost. It reads
    the kubeconfig you already have, watches every discovered resource through
    informers, and needs nothing installed in the cluster.
  caveats: |
    Run it with: kubectl spinoza --open
  platforms:
YAML
    krew_platform darwin arm64 "$darwin_arm64" spinoza
    krew_platform darwin amd64 "$darwin_amd64" spinoza
    krew_platform linux arm64 "$linux_arm64" spinoza
    krew_platform linux amd64 "$linux_amd64" spinoza
    krew_platform windows amd64 "$windows_amd64" spinoza.exe
    krew_platform windows arm64 "$windows_arm64" spinoza.exe
} > "$out/krew/spinoza.yaml"

cat > "$out/winget/Sophotech.Spinoza.yaml" <<YAML
PackageIdentifier: Sophotech.Spinoza
PackageVersion: $numeric
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
YAML

cat > "$out/winget/Sophotech.Spinoza.installer.yaml" <<YAML
PackageIdentifier: Sophotech.Spinoza
PackageVersion: $numeric
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
  - RelativeFilePath: spinoza.exe
    PortableCommandAlias: spinoza
Installers:
  - Architecture: x64
    InstallerUrl: $(url_of "$windows_amd64")
    InstallerSha256: $(hash_of "$windows_amd64")
  - Architecture: arm64
    InstallerUrl: $(url_of "$windows_arm64")
    InstallerSha256: $(hash_of "$windows_arm64")
ManifestType: installer
ManifestVersion: 1.6.0
YAML

cat > "$out/winget/Sophotech.Spinoza.locale.en-US.yaml" <<YAML
PackageIdentifier: Sophotech.Spinoza
PackageVersion: $numeric
PackageLocale: en-US
Publisher: Sophotech s.r.o.
PublisherUrl: https://sopho.tech
PackageName: Spinoza
PackageUrl: https://spinoza.tech
License: FSL-1.1-ALv2
LicenseUrl: https://github.com/$repo/blob/main/LICENSE
ShortDescription: Self-hosted Kubernetes GUI served on localhost
Tags:
  - kubernetes
  - k8s
  - devops
ManifestType: defaultLocale
ManifestVersion: 1.6.0
YAML

cat > "$out/chocolatey/spinoza.nuspec" <<XML
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd">
  <metadata>
    <id>spinoza</id>
    <version>$numeric</version>
    <title>Spinoza</title>
    <authors>Sophotech s.r.o.</authors>
    <owners>Sophotech s.r.o.</owners>
    <projectUrl>https://spinoza.tech</projectUrl>
    <packageSourceUrl>https://github.com/$repo</packageSourceUrl>
    <projectSourceUrl>https://github.com/$repo</projectSourceUrl>
    <docsUrl>https://github.com/$repo#readme</docsUrl>
    <bugTrackerUrl>https://github.com/$repo/issues</bugTrackerUrl>
    <licenseUrl>https://github.com/$repo/blob/main/LICENSE</licenseUrl>
    <requireLicenseAcceptance>false</requireLicenseAcceptance>
    <tags>kubernetes k8s devops gui</tags>
    <summary>Self-hosted Kubernetes GUI served on localhost</summary>
    <description>
      Spinoza serves a Kubernetes GUI from a single binary on localhost. It reads
      the kubeconfig you already have, watches every discovered resource through
      informers, and needs nothing installed in the cluster.
    </description>
  </metadata>
  <files>
    <file src="tools\\**" target="tools" />
  </files>
</package>
XML

cat > "$out/chocolatey/tools/chocolateyinstall.ps1" <<PS1
\$ErrorActionPreference = 'Stop'

\$packageArgs = @{
    packageName    = 'spinoza'
    unzipLocation  = Split-Path -Parent \$MyInvocation.MyCommand.Definition
    url64bit       = '$(url_of "$windows_amd64")'
    checksum64     = '$(hash_of "$windows_amd64")'
    checksumType64 = 'sha256'
}

Install-ChocolateyZipPackage @packageArgs
PS1

echo "render: wrote manifests for $version into $out"
