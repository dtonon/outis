set shell := ["bash", "-euo", "pipefail", "-c"]

pkg     := "github.com/dtonon/outis"
version := `sed -n 's/^const Number = "\(.*\)"/\1/p' internal/version/version.go`
commit  := `git rev-parse --short HEAD 2>/dev/null || true`
dirty   := `test -z "$(git status --porcelain 2>/dev/null)" || echo ".dirty"`
ldflags := "-X " + pkg + "/internal/version.Commit=" + commit + dirty

default: build

# Build ./outis with the commit hash injected
build:
    go build -ldflags "{{ldflags}}" -o outis ./cmd/outis

# Install into the Go bin directory
install:
    go install -ldflags "{{ldflags}}" ./cmd/outis

# Cross-compile into dist/ as outis-<version>-<os>-<arch>
dist:
    #!/usr/bin/env bash
    set -euo pipefail
    rm -rf dist && mkdir dist
    for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
        os="${target%/*}"; arch="${target#*/}"
        out="dist/outis-{{version}}-$os-$arch"
        [[ "$os" == windows ]] && out="$out.exe"
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w {{ldflags}}" -o "$out" ./cmd/outis
        echo "$out"
    done

# Vet and run the tests
test:
    go vet ./...
    go test ./...

# Print the version constant and the latest tag
version:
    @echo "code: {{version}}"
    @echo "tag:  $(git describe --tags --abbrev=0 2>/dev/null || echo none)"

# Bump the version, update the changelog, commit and tag (no push)
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail
    v="{{VERSION}}"
    [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "version must be X.Y.Z"; exit 1; }
    [[ -z "$(git status --porcelain)" ]] || { echo "working tree not clean"; exit 1; }
    ! git rev-parse -q --verify "refs/tags/v$v" >/dev/null || { echo "tag v$v exists"; exit 1; }
    grep -q '^## \[Unreleased\]' CHANGELOG.md || { echo "no Unreleased section in CHANGELOG.md"; exit 1; }
    sed -i '' "s/^const Number = \".*\"/const Number = \"$v\"/" internal/version/version.go
    sed -i '' "s/^## \[Unreleased\]/## [Unreleased]\n\n## [$v] - $(date +%Y-%m-%d)/" CHANGELOG.md
    just test
    git add internal/version/version.go CHANGELOG.md
    git commit -m "Release v$v"
    git tag -a "v$v" -m "v$v"
    echo "Released v$v, push with: git push --follow-tags"
