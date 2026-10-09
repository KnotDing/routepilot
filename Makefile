GO ?= go

.PHONY: build test bench verify fmt frontend frontend-test app package clean-artifacts
frontend: clean-artifacts
	cd web && npm run build
clean-artifacts:
	sh scripts/clean_artifacts.sh
frontend-test:
	cd web && npm test
build: frontend
	mkdir -p bin
	$(GO) build -trimpath -o bin/surge-controller ./cmd/controller
	$(GO) build -trimpath -o bin/dns-probe ./cmd/dns-probe
test:
	$(GO) test -race ./...
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./internal/cache
verify: frontend-test test
	$(GO) vet ./...
	$(MAKE) build
fmt:
	gofmt -w cmd internal tests
app: build
	sh scripts/build_app.sh
package: app
	sh scripts/package_macos.sh
