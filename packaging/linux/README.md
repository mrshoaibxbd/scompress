# scompress 2.4.2 — Linux x86-64

**A local-first image compressor with a standalone command-line tool and a native GTK 4 desktop app.**

> scompress by Shoaib. Your images are processed locally; they are not uploaded to a service.

## Install

1. Extract this ZIP archive.
2. Open a terminal in the extracted `scompress-2.4.2-linux-amd64` folder.
3. Make the scripts executable and install:

   ```bash
   chmod +x install.sh uninstall.sh
   ./install.sh
   ```

The installer places the executables in `~/.local/bin` and adds the GNOME Files and KDE Dolphin actions for the current user. It does not require `sudo` and does not install system packages.

### Desktop requirement

The graphical application requires **GTK 4.10 or newer**, a compatible glibc-based Linux distribution, and the host desktop's normal display libraries. The ready-to-install GUI bundle is built on Ubuntu 24.04; Ubuntu 24.04 or newer and compatible distributions are the intended targets. On Ubuntu or Debian:

```bash
sudo apt install libgtk-4-1
```

The command-line executable is statically linked. It does not require Go, Python, pip, ImageMagick, or libwebp to be installed at runtime.

## Start using scompress

Open the application from your desktop application menu as **scompress Image Compressor**, or run:

```bash
scompress-gui
```

You can also open the GUI with selected files:

```bash
scompress-gui ~/Downloads/photo.jpg
```

From the file manager, select one or more images and choose **Compress with scompress**. In GNOME Files, the action is under **Scripts**. In KDE Dolphin, it appears in the context menu.

## Command-line examples

```bash
# Compress to a maximum of 500 KB (500,000 bytes)
scompress photo.png --500

# Compress to 300 KB
scompress photo.jpg --300

# Convert PNG to WebP and target 500 KB
scompress photo.png --500 --format webp

# Process multiple JPEG images
scompress *.jpg --500

# Recursively process a directory
scompress ~/Pictures --500 --recursive --output ~/Pictures/Optimized

# Preview without writing files
scompress *.png --500 --dry-run

# Machine-readable report
scompress *.jpg --500 --json

# View every option
scompress --help
```

The original file is preserved by default. In-place replacement is optional in the GUI and requires explicit confirmation. Keep backups of important originals even when using replacement safeguards.

## Supported formats

| Format | Read | Write | Notes |
|---|---:|---:|---|
| JPEG / JPG | Yes | Yes | Quality-based compression |
| PNG | Yes | Yes | Palette optimization available |
| GIF | Yes | Yes | Animated GIF input/output supported |
| WebP | Yes | Yes | Static WebP only |
| Animated WebP | No | No | Not supported in this release |
| AVIF | No | No | Not supported in this release |

## Uninstall

From this extracted package directory, run:

```bash
./uninstall.sh
```

This removes the scompress executables and per-user desktop integrations. It does not remove GTK, other system packages, or your image files.

## Package contents

- `dist/linux-amd64/scompress` — standalone CLI executable.
- `dist/linux-amd64/scompress-gui` — native GTK 4 desktop executable.
- `install.sh` / `uninstall.sh` — per-user installation and removal.
- `USER_GUIDE.md` — fuller usage and troubleshooting guide.
- `LICENSE` and `THIRD_PARTY_NOTICES.txt` — licensing information.
- `CHANGELOG.md` and `VERSION` — release information.

For source code, development instructions, contribution guidelines, and issue reporting, see the project repository.
