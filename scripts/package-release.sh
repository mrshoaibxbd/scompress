#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '\r\n' < VERSION)"
TAG="${1:-v${VERSION}}"
TAG_VERSION="${TAG#v}"

if [[ "$TAG_VERSION" != "$VERSION" ]]; then
  echo "Release tag $TAG does not match VERSION $VERSION." >&2
  exit 1
fi

CLI="$ROOT/dist/linux-amd64/scompress"
GUI="$ROOT/dist/linux-amd64/scompress-gui"
[[ -x "$CLI" ]] || { echo "Missing executable: $CLI (run ./build.sh first)" >&2; exit 1; }
[[ -x "$GUI" ]] || { echo "Missing executable: $GUI (run ./build.sh first)" >&2; exit 1; }
[[ "$("$CLI" --version)" == "scompress $VERSION" ]] || {
  echo "CLI version does not match VERSION ($VERSION)." >&2
  exit 1
}

for required in \
  install.sh uninstall.sh LICENSE THIRD_PARTY_NOTICES.txt CHANGELOG.md \
  integrations/nautilus/"Compress with scompress" \
  share/icons/hicolor/scalable/apps/scompress.svg \
  packaging/linux/README.md packaging/linux/USER_GUIDE.md \
  packaging/linux/INSTALL.txt packaging/linux/UNINSTALL.txt \
  packaging/linux/PACKAGE_CONTENTS.txt; do
  [[ -f "$required" ]] || { echo "Required release file is missing: $required" >&2; exit 1; }
done

RELEASE_DIR="$ROOT/release"
PACKAGE_NAME="scompress-${VERSION}-linux-amd64"
STAGE="$RELEASE_DIR/$PACKAGE_NAME"
mkdir -p "$RELEASE_DIR"
rm -rf -- "$STAGE"
mkdir -p \
  "$STAGE/dist/linux-amd64" \
  "$STAGE/integrations/nautilus" \
  "$STAGE/share/icons/hicolor/scalable/apps"

install -m 0755 "$CLI" "$STAGE/dist/linux-amd64/scompress"
install -m 0755 "$GUI" "$STAGE/dist/linux-amd64/scompress-gui"
install -m 0755 install.sh "$STAGE/install.sh"
install -m 0755 uninstall.sh "$STAGE/uninstall.sh"
install -m 0755 "integrations/nautilus/Compress with scompress" \
  "$STAGE/integrations/nautilus/Compress with scompress"
install -m 0644 share/icons/hicolor/scalable/apps/scompress.svg \
  "$STAGE/share/icons/hicolor/scalable/apps/scompress.svg"
install -m 0644 LICENSE THIRD_PARTY_NOTICES.txt CHANGELOG.md VERSION "$STAGE/"
install -m 0644 packaging/linux/README.md "$STAGE/README.md"
install -m 0644 packaging/linux/USER_GUIDE.md "$STAGE/USER_GUIDE.md"
install -m 0644 packaging/linux/INSTALL.txt "$STAGE/INSTALL.txt"
install -m 0644 packaging/linux/UNINSTALL.txt "$STAGE/UNINSTALL.txt"
install -m 0644 packaging/linux/PACKAGE_CONTENTS.txt "$STAGE/PACKAGE_CONTENTS.txt"

ZIP="$RELEASE_DIR/${PACKAGE_NAME}.zip"
TAR="$RELEASE_DIR/${PACKAGE_NAME}.tar.gz"
rm -f -- "$ZIP" "$TAR"
(
  cd "$RELEASE_DIR"
  zip -qr "$(basename "$ZIP")" "$PACKAGE_NAME"
  tar -czf "$(basename "$TAR")" "$PACKAGE_NAME"
)

unzip -t "$ZIP" >/dev/null
tar -tzf "$TAR" >/dev/null

printf 'Created ready-to-install release assets:\n  %s\n  %s\n' "$ZIP" "$TAR"
printf '\nPackage includes:\n'
find "$STAGE" -type f -printf '  %P\n' | sort
