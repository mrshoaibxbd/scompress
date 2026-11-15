# Contributing to scompress

Thanks for considering a contribution.

## Before opening an issue or pull request

- Search existing issues and pull requests to avoid duplicates.
- For a bug report, include the scompress version, Linux distribution, architecture, desktop session (Wayland or X11), GTK version, image format, and the exact command or GUI operation.
- Do not attach private or sensitive images. A small synthetic test image is usually sufficient.
- Never include personal files, EXIF/GPS data, credentials, or private paths in logs.

## Development setup

On Ubuntu/Debian:

```bash
sudo apt update
sudo apt install golang-go build-essential pkg-config libwebp-dev libgtk-4-dev
```

Then run:

```bash
./build.sh
```

The test and quality checks are:

```bash
CGO_ENABLED=1 go test ./cmd/scompress
CGO_ENABLED=1 go vet ./cmd/scompress
gofmt -l cmd/scompress
bash -n build.sh build-and-install.sh install.sh uninstall.sh
```

The GUI can be run from the build output with:

```bash
./dist/linux-amd64/scompress-gui
```

## Pull request expectations

- Keep changes focused and explain the user-visible impact.
- Add or update tests for behavior changes in the Go engine.
- Preserve the rule that normal compression never overwrites original files.
- Treat in-place replacement and metadata handling as safety-sensitive areas; explain the failure modes and test recovery behavior.
- Update `README.md` and `CHANGELOG.md` when commands, options, formats, or user-visible behavior change.
- Run `./build.sh` on Linux x86-64 before submitting when changing C, GTK, cgo, or build scripts.
- Do not commit generated files from `dist/linux-amd64/`.

## Code style

- Format Go code with `gofmt`.
- Use clear C naming and check GTK/GLib API compatibility against the documented minimum GTK version.
- Keep shell scripts compatible with Bash and quote paths that may contain spaces.
