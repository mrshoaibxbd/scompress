## Summary

Describe the change and the user-visible impact.

## Validation

- [ ] `CGO_ENABLED=1 go test ./cmd/scompress`
- [ ] `CGO_ENABLED=1 go vet ./cmd/scompress`
- [ ] `./build.sh` (when changing C, GTK, cgo, or build scripts)
- [ ] `bash -n build.sh build-and-install.sh install.sh uninstall.sh`
- [ ] Documentation and changelog updated where applicable

## Safety review

- [ ] Normal compression still preserves original files.
- [ ] Replacement behavior and rollback paths were reviewed (if changed).
- [ ] No private images, paths, or metadata are included in the PR.
