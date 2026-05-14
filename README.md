# gh-peek

A terminal UI for browsing GitHub Actions runs, jobs, and logs from
inside a local git repository. Built with Go and
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea).

Open it from a checkout and it picks the right starting view:

- on the default branch — the repo-wide all-runs list
- on a branch with an active PR — the PR's runs
- on a branch without a PR — the branch's runs

## Features

- Runs list with semantic status badges, ETag-aware polling, active-only
  filter, substring search, and a `b` view-cycle (branch / PR / all).
- Run detail with parallel jobs/steps panes, focus toggle, and live
  refresh while the run is active.
- Job log viewer with vim-style navigation (`g`/`G`), live `/` search,
  wrap toggle, "jump to first failure" (`F`), and a 10 MB tail-capped
  in-memory buffer that preserves ANSI for rendering and strips it for
  search.
- `o` opens the focused run / job in the system browser.
- Releases browser (`L` from the runs list) with status badges
  (`latest` / `pre` / `draft`), substring search, release notes and
  asset table in the detail view, and `enter` on an asset opens its
  download URL in the browser.
- Packages browser (`P` from the runs list) covering all six GitHub
  Packages types (container, npm, maven, rubygems, nuget, docker),
  with substring search, semantic type badges, and a per-package
  versions list (TAGS column shown for container/docker).
- Auto-refresh on visible screens with active runs, paused while typing
  in a search input.
- Friendly bootstrap errors (`gh` missing, not a git repo, no GitHub
  remote, detached HEAD).

## Install

### As a `gh` extension

```sh
gh extension install sud0whoami/gh-peek
gh peek
```

### Pre-built binary (GitHub Releases)

Pre-built binaries for Linux, macOS, and Windows are attached to every
[GitHub Release](https://github.com/sud0whoami/gh-peek/releases).

**Quick install (Linux / macOS):**

```sh
curl -fsSL https://raw.githubusercontent.com/sud0whoami/gh-peek/main/install.sh | sh
```

**Step-by-step (replace `VERSION` with the desired release, e.g. `1.0.0`):**

```sh
VERSION="1.0.0"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac
ARCHIVE="gh-peek_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/sud0whoami/gh-peek/releases/download/v${VERSION}"
curl -fsSL "${BASE}/${ARCHIVE}" -o "/tmp/${ARCHIVE}"
curl -fsSL "${BASE}/checksums.txt" | grep "${ARCHIVE}" | shasum -a 256 -c -
tar -xzf "/tmp/${ARCHIVE}" -C /tmp gh-peek
sudo install -m 755 /tmp/gh-peek /usr/local/bin/
rm -f "/tmp/${ARCHIVE}" /tmp/gh-peek
```

### With Go

```sh
go install github.com/sud0whoami/gh-peek/cmd/gh-peek@latest
gh-peek
```

This downloads the module via the Go module proxy and compiles it
locally; it does not check out a working tree.

### From source

```sh
git clone https://github.com/sud0whoami/gh-peek.git
cd gh-peek
make build
./bin/gh-peek
```

### Local `gh` extension build (development)

```sh
make extension
gh extension install .
gh peek
```

## Usage

Run inside any git repository with a GitHub remote:

```sh
gh-peek    # standalone (built via `make build`)
gh peek    # as a gh extension
```

The binary is interactive only — running it with stdout redirected
prints a one-line message and exits with code 2.

### Open in browser

`--web` is a non-interactive shortcut that resolves the current
repository and opens its GitHub Actions page in the system browser:

```sh
gh-peek --web
gh peek --web
```

## Keys

The full key map lives in
[internal/ui/keymap](internal/ui/keymap/keymap.go). Highlights:

- Runs list: `↑/↓` move · `enter` open · `/` search · `a` active-only ·
  `b` cycle view · `r` refresh · `R` toggle auto-refresh · `o` browser ·
  `L` releases · `P` packages · `?` help · `q` quit
- Run detail: `↑/↓`/`j`/`k` move · `tab` switch pane · `enter` open job log · `o` browser ·
  `r` refresh · `R` toggle auto-refresh · `?` help · `esc`/`b` back · `q` quit
- Log viewer: `↑/↓`/`j`/`k` cursor · `PgUp`/`PgDn` page · `g`/`G` top/bottom ·
  `enter`/`space` toggle node · `→`/`l` expand · `←`/`h` collapse · `E` expand all ·
  `O` collapse all · `t` timestamps · `v` cycle mode (outline/compact/raw) ·
  `/` search · `n`/`N` next/prev · `w` wrap · `F` first failure ·
  `r` refresh · `R` toggle auto-refresh · `o` browser · `?` help · `esc`/`b` back · `q` quit
- Releases list: `↑/↓` move · `enter` open · `/` search · `r` refresh ·
  `R` toggle auto-refresh · `o` browser · `esc`/`b`/`L` back to runs · `?` help · `q` quit
- Release detail: `↑/↓`/`j`/`k` scroll / move · `tab` switch pane (notes / assets) ·
  `enter` open focused asset in browser · `o` open release page ·
  `r` refresh · `R` toggle auto-refresh · `?` help · `esc`/`b` back · `q` quit
- Packages list: `↑/↓` move · `enter` open · `/` search · `r` refresh ·
  `R` toggle auto-refresh · `o` browser · `esc`/`b`/`P` back to runs · `?` help · `q` quit
- Package detail: `↑/↓` move · `o` open focused version · `O` open package page ·
  `r` refresh · `R` toggle auto-refresh · `?` help · `esc`/`b` back · `q` quit

## Configuration

- Authentication: uses the `gh` CLI's stored credentials by default;
  honors `GH_TOKEN` / `GITHUB_TOKEN` and `GH_HOST`.
- Color: respects `NO_COLOR` and `CLICOLOR`. `GH_FORCE_TTY` is honored
  for TTY detection.
- TTY: interactive only. Non-TTY stdout exits with code 2.

## Development

```sh
make test         # quick test run
make test-fresh   # disable test cache (go test -count=1)
make test-race    # race detector
make build        # builds bin/gh-peek
make extension    # builds ./gh-peek for `gh extension install .`
make check        # gofmt + tests
```

CI runs `gofmt`, `go vet`, `golangci-lint`, `go build`, and the
race-enabled test suite on every push and pull request — see
[.github/workflows/ci.yml](.github/workflows/ci.yml).

Releases are automated via [GoReleaser](https://goreleaser.com): pushing
a `v*` tag triggers
[.github/workflows/release.yml](.github/workflows/release.yml), which
builds cross-platform archives + checksums and creates a draft GitHub
Release. See [.goreleaser.yaml](.goreleaser.yaml) for the build matrix.

## Security

See [SECURITY.md](SECURITY.md) for how to report a vulnerability.

## License

[MIT](LICENSE)
