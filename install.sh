#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
VERSION="$(tr -d '\r\n' < "$ROOT/VERSION")"
OUT="$ROOT/dist/linux-amd64"
CLI="$OUT/scompress"
GUI="$OUT/scompress-gui"
[[ -x "$CLI" && -x "$GUI" ]] || { echo "The scompress CLI and GUI binaries were not found. Build from source with ./build.sh, or download the ready-to-install release package." >&2; exit 1; }

case "$(uname -m)" in x86_64|amd64) ;; *) echo "This release supports Linux x86-64 only." >&2; exit 1 ;; esac

# The CLI is standalone; the native GUI uses the host GTK 4 desktop runtime.
if command -v ldd >/dev/null 2>&1; then
  missing_gui_libs="$(ldd "$GUI" 2>/dev/null | awk '/not found/ { print $1 }')"
  if [[ -n "$missing_gui_libs" ]]; then
    printf 'The scompress GUI is missing these runtime libraries:\n%s\n\n' "$missing_gui_libs" >&2
    echo "Install GTK 4 and the missing desktop libraries, then run ./install.sh again." >&2
    echo "Ubuntu/Debian: sudo apt install libgtk-4-1" >&2
    exit 1
  fi
fi

BIN_DIR="$HOME/.local/bin"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
NAUTILUS_DIR="$DATA_HOME/nautilus/scripts"
DOLPHIN_DIR="$DATA_HOME/kio/servicemenus"
ICON_DIR="$DATA_HOME/icons/hicolor/scalable/apps"
APP_DIR="$DATA_HOME/applications"
mkdir -p "$BIN_DIR" "$NAUTILUS_DIR" "$DOLPHIN_DIR" "$ICON_DIR" "$APP_DIR"

install_atomic() {
  local src="$1" dst="$2" mode="$3" tmp
  tmp="$(mktemp "${dst}.XXXXXX")"
  if ! install -m "$mode" "$src" "$tmp"; then rm -f -- "$tmp"; return 1; fi
  if ! mv -f -- "$tmp" "$dst"; then rm -f -- "$tmp"; return 1; fi
}

install_atomic "$CLI" "$BIN_DIR/scompress" 0755
install_atomic "$GUI" "$BIN_DIR/scompress-gui" 0755
install_atomic "$ROOT/integrations/nautilus/Compress with scompress" "$NAUTILUS_DIR/Compress with scompress" 0755
install_atomic "$ROOT/share/icons/hicolor/scalable/apps/scompress.svg" "$ICON_DIR/scompress.svg" 0644

GUI_EXEC="$BIN_DIR/scompress-gui"
# Desktop Entry quoting: escape backslashes and double quotes inside the quoted executable path.
GUI_ESCAPED="${GUI_EXEC//\\/\\\\}"
GUI_ESCAPED="${GUI_ESCAPED//\"/\\\"}"
DOLPHIN_TMP="$(mktemp "$DOLPHIN_DIR/scompress.desktop.XXXXXX")"
cat > "$DOLPHIN_TMP" <<DESKTOP
[Desktop Entry]
Type=Service
MimeType=image/jpeg;image/png;image/gif;image/webp;
Actions=scompress;
X-KDE-Priority=TopLevel

[Desktop Action scompress]
Name=Compress with scompress
Icon=scompress
Exec="$GUI_ESCAPED" %F
DESKTOP
chmod 0644 "$DOLPHIN_TMP"
mv -f -- "$DOLPHIN_TMP" "$DOLPHIN_DIR/scompress.desktop"

APP_TMP="$(mktemp "$APP_DIR/com.shoaib.scompress.desktop.XXXXXX")"
cat > "$APP_TMP" <<DESKTOP
[Desktop Entry]
Type=Application
Name=scompress Image Compressor
Comment=Compress images locally with scompress
Icon=scompress
Exec="$GUI_ESCAPED"
Terminal=false
Categories=Graphics;Utility;
StartupNotify=true
DESKTOP
chmod 0644 "$APP_TMP"
mv -f -- "$APP_TMP" "$APP_DIR/com.shoaib.scompress.desktop"

# Retire the previous custom X11/GJS launchers if present.
rm -f -- "$DATA_HOME/nautilus/scripts/Compress with scompress (old)" \
         "$DATA_HOME/scompress/ui/scompress-dialog.js"
rmdir --ignore-fail-on-non-empty "$DATA_HOME/scompress/ui" 2>/dev/null || true
rmdir --ignore-fail-on-non-empty "$DATA_HOME/scompress" 2>/dev/null || true

if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$APP_DIR" >/dev/null 2>&1 || true; fi

cat <<INFO

scompress $VERSION installed.
CLI:       $BIN_DIR/scompress
GUI:       $BIN_DIR/scompress-gui
GNOME:     right-click images -> Scripts -> Compress with scompress
KDE:       right-click images -> Compress with scompress
App menu:  scompress Image Compressor

The GTK 4 runtime is required for the GUI.
Install your distribution's GTK 4 runtime package if it is missing.
The standalone CLI does not require GTK.
INFO
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "Add to PATH if needed: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;; esac
