#!/usr/bin/env bash
set -euo pipefail
BIN_DIR="$HOME/.local/bin"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
rm -f -- "$BIN_DIR/scompress" "$BIN_DIR/scompress-gui" \
  "$DATA_HOME/nautilus/scripts/Compress with scompress" \
  "$DATA_HOME/kio/servicemenus/scompress.desktop" \
  "$DATA_HOME/icons/hicolor/scalable/apps/scompress.svg" \
  "$DATA_HOME/applications/com.shoaib.scompress.desktop" \
  "$DATA_HOME/scompress/ui/scompress-dialog.js"
rmdir --ignore-fail-on-non-empty "$DATA_HOME/scompress/ui" 2>/dev/null || true
rmdir --ignore-fail-on-non-empty "$DATA_HOME/scompress" 2>/dev/null || true
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$DATA_HOME/applications" >/dev/null 2>&1 || true; fi
echo "scompress executables and desktop integrations removed. System packages and user images were not touched."
