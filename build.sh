#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

case "$(uname -m)" in
  x86_64|amd64) ;;
  *) echo "This source build currently targets Linux x86-64 only." >&2; exit 1 ;;
esac

VERSION="$(tr -d '\r\n' < VERSION)"
[[ -n "$VERSION" ]] || { echo "VERSION file is empty." >&2; exit 1; }
grep -Fq "version           = \"$VERSION\"" cmd/scompress/main.go || {
  echo "Go CLI version does not match VERSION ($VERSION)." >&2; exit 1;
}
grep -Fq "#define APP_VERSION \"$VERSION\"" ui/scompress-gui.c || {
  echo "GTK GUI version does not match VERSION ($VERSION)." >&2; exit 1;
}

missing=()
command -v go >/dev/null 2>&1 || missing+=(golang-go)
command -v cc >/dev/null 2>&1 || missing+=(build-essential)
command -v pkg-config >/dev/null 2>&1 || missing+=(pkg-config)
pkg-config --atleast-version=4.10 gtk4 2>/dev/null || missing+=(libgtk-4-dev)
pkg-config --exists libwebp 2>/dev/null || missing+=(libwebp-dev)

if ((${#missing[@]})); then
  printf 'Build prerequisites are missing. On Ubuntu/Debian, install them with:\n\n  sudo apt update && sudo apt install golang-go build-essential pkg-config libwebp-dev libgtk-4-dev\n\n' >&2
  exit 1
fi

WEBP_LIBDIR="$(pkg-config --variable=libdir libwebp)"
WEBP_ARCHIVES_FOUND=0
for libdir in "$WEBP_LIBDIR" /usr/lib/x86_64-linux-gnu /usr/lib64; do
  if [[ -f "$libdir/libwebp.a" && -f "$libdir/libsharpyuv.a" ]]; then
    WEBP_ARCHIVES_FOUND=1
    break
  fi
done
if [[ "$WEBP_ARCHIVES_FOUND" -ne 1 ]]; then
  echo "Static libwebp archives (libwebp.a and libsharpyuv.a) were not found. Install/reinstall libwebp-dev." >&2
  exit 1
fi

OUT="$ROOT/dist/linux-amd64"
mkdir -p "$OUT"

printf '\n==> Running Go tests\n'
CGO_ENABLED=1 go test ./cmd/scompress

printf '\n==> Running Go vet\n'
CGO_ENABLED=1 go vet ./cmd/scompress

printf '\n==> Building standalone Go compression engine (%s)\n' "$VERSION"
CGO_ENABLED=1 go build \
  -trimpath \
  -ldflags='-s -w -buildid= -linkmode external -extldflags "-static"' \
  -o "$OUT/scompress" \
  ./cmd/scompress
chmod 0755 "$OUT/scompress"

printf '\n==> Building native GTK 4 desktop interface (%s)\n' "$VERSION"
cc -std=c11 -O2 -pipe -Wall -Wextra -Wformat=2 \
  $(pkg-config --cflags gtk4) \
  -o "$OUT/scompress-gui" \
  ui/scompress-gui.c \
  $(pkg-config --libs gtk4)
chmod 0755 "$OUT/scompress-gui"

printf '\n==> Checking build outputs\n'
file "$OUT/scompress" "$OUT/scompress-gui"
CLI_VERSION="$("$OUT/scompress" --version)"
[[ "$CLI_VERSION" == "scompress $VERSION" ]] || {
  echo "Unexpected CLI version: $CLI_VERSION" >&2; exit 1;
}

CLI_LINKAGE="$(ldd "$OUT/scompress" 2>&1 || true)"
if ! grep -q "not a dynamic executable" <<< "$CLI_LINKAGE"; then
  printf 'The CLI is expected to be statically linked, but ldd reported:\n%s\n' "$CLI_LINKAGE" >&2
  exit 1
fi
printf '\nCLI linkage: static (no shared-library dependencies)\n'

GUI_LINKAGE="$(ldd "$OUT/scompress-gui" 2>&1 || true)"
if ! grep -q 'libgtk-4\.so' <<< "$GUI_LINKAGE"; then
  printf 'The GUI does not appear to link against GTK 4:\n%s\n' "$GUI_LINKAGE" >&2
  exit 1
fi
printf '\nGUI linkage (GTK is provided by the desktop runtime):\n'
grep -E 'libgtk-4|libgio-2.0|libglib-2.0|libwayland-client|libX11' <<< "$GUI_LINKAGE" | head -12 || true

printf '\nBuild completed successfully: scompress %s\n' "$VERSION"
printf 'Next: ./install.sh\n'
