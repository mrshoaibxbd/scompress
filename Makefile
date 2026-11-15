.PHONY: test vet fmt-check build install uninstall clean

test:
	CGO_ENABLED=1 go test ./cmd/scompress

vet:
	CGO_ENABLED=1 go vet ./cmd/scompress

fmt-check:
	@test -z "$$(gofmt -l cmd/scompress)" || { gofmt -l cmd/scompress; exit 1; }

build:
	./build.sh

install: build
	./install.sh

uninstall:
	./uninstall.sh

clean:
	rm -f dist/linux-amd64/scompress dist/linux-amd64/scompress-gui
