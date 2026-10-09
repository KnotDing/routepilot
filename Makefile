GO ?= go

.PHONY: build test bench verify verify-release native-test ui-test coverage fuzz package-check checksums fmt frontend frontend-test app package clean-artifacts
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
	$(GO) test -race -count=1 ./...
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./internal/cache
verify: frontend-test test
	$(GO) vet ./...
	$(MAKE) build
# Run in order: rebuilding the frontend removes previous package artifacts.
verify-release: verify
	$(MAKE) native-test ui-test
	$(MAKE) package
	$(MAKE) package-check
native-test:
	sh scripts/test_native.sh
ui-test:
	node scripts/ui_forms_check.cjs
coverage:
	mkdir -p .cache
	$(GO) test -race -count=1 -coverprofile=.cache/release-coverage.out ./...
	$(GO) tool cover -func=.cache/release-coverage.out
fuzz:
	$(GO) test -run '^$$' -fuzz '^FuzzVersionParser$$' -fuzztime=5s -parallel=2 ./internal/softwareupdate
checksums:
	rm -f artifacts/package-verification.json
	cd artifacts && shasum -a 256 RoutePilot-macos-$$(uname -m).dmg RoutePilot-macos-$$(uname -m).zip surge-controller-macos-$$(uname -m).tar.gz > SHA256SUMS
package-check: checksums
	python3 scripts/check_package_test.py
	python3 scripts/check_package.py
fmt:
	gofmt -w cmd internal tests
app: build
	sh scripts/build_app.sh
package: app
	sh scripts/package_macos.sh
