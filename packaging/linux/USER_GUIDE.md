# scompress user guide

## 1. What is scompress?

scompress is a Linux image-compression utility with two ways to work:

- **Desktop application:** a GTK 4 interface for selecting images and adjusting settings.
- **Command line:** a fast tool for scripts, batch jobs, and terminal workflows.

Image processing is local. scompress does not upload image contents to a remote server.

## 2. Desktop application

Launch from the application menu (**scompress Image Compressor**) or run:

```bash
scompress-gui
```

You can pass image paths to open them in the application:

```bash
scompress-gui ~/Pictures/photo.jpg ~/Pictures/diagram.png
```

### File-manager actions

- **GNOME Files:** select image files, right-click, then choose **Scripts → Compress with scompress**.
- **KDE Dolphin:** select image files, right-click, then choose **Compress with scompress**.

The file-manager action opens the graphical application with the selected files.

### Suggested workflow

1. Add one or more images.
2. Choose a target maximum size, such as 500 KB.
3. Choose whether to keep the original format or convert to another supported format.
4. Adjust quality or dimensions if needed.
5. Review metadata and replacement settings.
6. Start compression and inspect the result summary.

Original files are preserved by default. Enable replacement only when you intentionally want to replace the selected originals. Keep separate backups of valuable images.

## 3. Command-line usage

The target uses decimal kilobytes: `--500` means 500,000 bytes.

```bash
scompress image.png --500
```

The compressed copy is written beside the original using a `_compressed` suffix, unless an output directory is specified. Existing files are not silently overwritten.

### Useful commands

```bash
# Target 250 KB
scompress photo.jpg --250

# Set preferred quality
scompress photo.jpg --500 --quality 85

# Keep the source dimensions
scompress photo.jpg --500 --no-resize

# Set dimension limits
scompress photo.jpg --500 --max-width 1920 --max-height 1080

# Convert format
scompress image.png --500 --format webp

# Send output to a folder
scompress photo.jpg --500 --output ~/Pictures/Optimized

# Process a directory recursively
scompress ~/Pictures --500 --recursive --output ~/Pictures/Optimized

# Preview only
scompress *.jpg --500 --dry-run

# Emit JSON for scripts
scompress *.jpg --500 --json

# Display usage
scompress --help
```

Run `scompress --help` for the complete options available in the installed build.

## 4. Formats and limitations

| Format | Input | Output | Details |
|---|---:|---:|---|
| JPEG / JPG | Yes | Yes | Lossy quality control |
| PNG | Yes | Yes | Palette optimization can reduce colors |
| GIF | Yes | Yes | Animated GIF is supported |
| WebP | Yes | Yes | Static WebP only |
| Animated WebP | No | No | Unsupported |
| AVIF | No | No | Unsupported |

Metadata preservation is format-dependent. `--keep-metadata` supports selected JPEG and PNG metadata; it does not preserve metadata across arbitrary format conversions.

## 5. Privacy and replacement safety

The default metadata policy removes identifying metadata where supported. This can remove camera details and GPS coordinates from output files. Metadata behavior depends on the source format and conversion path.

Original files are preserved by default. If replacement is enabled in the GUI, scompress stages output and checks the source files before applying changes. Recovery mechanisms are best-effort and are not a substitute for a separate backup.

## 6. Troubleshooting

### `scompress: command not found`

Make sure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Add that line to `~/.bashrc` or the startup file for your shell if needed.

### The GUI does not start

The desktop app requires GTK 4.10 or newer and compatible Linux desktop libraries. On Ubuntu/Debian, install the GTK runtime:

```bash
sudo apt install libgtk-4-1
```

If the message names another missing shared library, install the matching runtime package for your distribution. The CLI can still run independently of the GUI runtime.

### GNOME Files action is not visible

The Nautilus action is installed under the user Scripts menu. Close and reopen GNOME Files. The menu item is under **Scripts**, not necessarily in the top-level context menu.

### Dolphin action is not visible

Restart Dolphin or refresh its service menus. Confirm that the service menu file exists at:

```text
~/.local/share/kio/servicemenus/scompress.desktop
```

## 7. Uninstall

From the extracted release directory:

```bash
./uninstall.sh
```

The uninstaller removes scompress binaries and per-user desktop integration files. It leaves system packages and personal files untouched.
