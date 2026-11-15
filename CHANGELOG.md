# Changelog

All notable changes to scompress are documented here.

## [2.4.2] - 2026-11-15

### Final engineering and release polish

- Replaced the simulated random percentage ticker in the GUI activity monitor with truthful per-file completion counts.
- The activity spinner remains active while the current file is being processed; the counter advances only after a file operation returns.
- Kept the established GTK 4 interface and the Go compression engine behavior intact.
- Added a professional GitHub repository structure: CI and tagged-release workflows, contribution guidance, security policy, support guidance, issue templates, and pull-request template.
- Added an MIT license for the scompress project and retained the separate libwebp BSD 3-Clause notice.
- Corrected the source repository packaging so generated executables are excluded and the README accurately describes the source checkout.
- Added a designated screenshot path for the maintainer's final desktop capture.
- Added a ready-to-install Linux x86-64 release bundle containing the CLI, GTK GUI, installer, uninstaller, package README, and a fuller user guide.
- Added a reusable release-packaging script and CI checks for the contents of the binary release archive.
- Updated CI and tagged release builds to use the supported Go 1.27 toolchain.
- Added an installer preflight check that reports unresolved GUI shared-library dependencies before installing.

### Build and validation

- Go tests and `go vet` are part of CI.
- CI builds the Linux x86-64 CLI and GTK 4 GUI on Ubuntu 24.04.
- Tagged releases produce a Linux x86-64 install bundle, source archive, and SHA256 checksums.

## [2.4.1] - 2026-11-15

### GTK 4 build compatibility hotfix

- Replaced unsupported GTK 3-era `gtk_window_set_size_request()` with GTK 4 `gtk_widget_set_size_request()`.
- Replaced unsupported `gtk_entry_set_width_chars()` with the GTK 4 `gtk_editable_set_width_chars()` interface method.
- Uses `gtk_css_provider_load_from_string()` on GTK 4.12+ and retains the `load_from_data()` fallback for GTK 4.10–4.11.
- Retains the minimum supported GTK version at 4.10.

## [2.4.0] - 2026-11-15

### Native desktop application

- Replaced the custom X11-drawn GUI with a native GTK 4 application.
- Uses GTK 4 through the system development/runtime packages; no GJS or Python is used.
- Uses GTK's display backend, supporting Wayland and X11/XWayland desktops.
- Added native file selection, output-folder selection, quality control, dimension limits, metadata controls, presets, background compression, and a results panel.
- Kept original replacement opt-in, confirmation-gated, staged, source-checked, and backed by same-directory recovery hard links before the replacement commit.
- Added an application launcher and retained Nautilus and Dolphin integrations.
