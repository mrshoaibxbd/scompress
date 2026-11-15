# Build details

## Supported build target

The current release targets Linux x86-64 only.

## Dependencies

Ubuntu/Debian:

```bash
sudo apt update
sudo apt install golang-go build-essential pkg-config libwebp-dev libgtk-4-dev
```

- Go 1.23+
- GCC / C compiler
- `pkg-config`
- GTK 4.10+ headers and libraries
- libwebp headers and static archives (`libwebp.a`, `libsharpyuv.a`)

## Build

```bash
./build.sh
```

Outputs:

```text
dist/linux-amd64/scompress
 dist/linux-amd64/scompress-gui
```

The CLI statically links libwebp and is checked for dynamic dependencies. The GUI is linked against the host GTK 4, GLib, and display-stack libraries so it integrates with the installed desktop session.

## Install and uninstall

```bash
./install.sh
./uninstall.sh
```

The scripts only manage the per-user installation in `~/.local` (or `$XDG_DATA_HOME`). They do not install or remove system packages.

## Tests

```bash
CGO_ENABLED=1 go test ./cmd/scompress
CGO_ENABLED=1 go vet ./cmd/scompress
gofmt -l cmd/scompress
bash -n build.sh build-and-install.sh install.sh uninstall.sh
```
