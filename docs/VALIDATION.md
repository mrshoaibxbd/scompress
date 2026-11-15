# Validation record for 2.4.2

## Checks completed during source preparation

- `gofmt` check passed.
- `CGO_ENABLED=1 go test ./cmd/scompress` passed.
- `CGO_ENABLED=1 go test -race ./cmd/scompress` passed.
- `CGO_ENABLED=1 go vet ./cmd/scompress` passed.
- Linux x86-64 CLI built successfully with static libwebp linking.
- The CLI reported `scompress 2.4.2` and `ldd` reported `not a dynamic executable`.
- `bash -n` passed for all build, install, and uninstall scripts.
- YAML parsing passed for GitHub Actions, Dependabot, and issue templates.
- CI includes a temporary-home install/uninstall smoke test for the generated CLI, GUI, Nautilus script, and Dolphin service entry.
- The GUI activity monitor no longer contains the randomized percentage ticker; it displays the number of completed file operations while the spinner indicates ongoing work.

## Check still required in CI / on a GTK development system

The source-preparation container used for this release does not have GTK 4 development headers, and installing them was unavailable during preparation. Therefore, the updated C/GTK GUI was not compiled in this container. The GitHub Actions CI workflow is configured to compile the GUI on Ubuntu 24.04 with GTK 4 development packages. The maintainer should also run `./build.sh` and test the GUI under the target Wayland or X11 desktop before publishing a tagged release.
