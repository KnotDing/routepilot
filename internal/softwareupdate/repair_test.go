package softwareupdate

import (
	"runtime"
	"testing"

	"surge-controller/internal/buildinfo"
)

func TestBundledAppRequiresRoutePilotServiceExecutable(t *testing.T) {
	app, err := bundledApp("/Applications/RoutePilot.app/Contents/Resources/Service/bin/surge-controller")
	if err != nil || app != "/Applications/RoutePilot.app" {
		t.Fatalf("bundled service executable rejected: app %q, error %v", app, err)
	}
	for _, path := range []string{
		"/Applications/RoutePilot.app/Contents/Resources/Service/bin/routepilot",
		"/Applications/Other.app/Contents/Resources/Service/bin/surge-controller",
		"/Applications/RoutePilot.app/Contents/MacOS/RoutePilot",
		"/tmp/surge-controller",
	} {
		if _, err := bundledApp(path); err == nil {
			t.Fatalf("accepted replacement source %s", path)
		}
	}
}

func TestBundledBackendMustMatchAppMetadata(t *testing.T) {
	matching := buildinfo.Info{Name: buildinfo.Current().Name, Version: "1.0.1-beta.3", Build: "59", OS: runtime.GOOS, Arch: runtime.GOARCH}
	if err := bundledBackendMatches(matching, "1.0.1-beta.3", "59"); err != nil {
		t.Fatalf("matching bundled backend rejected: %v", err)
	}
	for _, info := range []buildinfo.Info{
		{Name: buildinfo.Current().Name, Version: "1.0.1-beta.2", Build: "59", OS: runtime.GOOS, Arch: runtime.GOARCH},
		{Name: buildinfo.Current().Name, Version: "1.0.1-beta.3", Build: "58", OS: runtime.GOOS, Arch: runtime.GOARCH},
		{Name: buildinfo.Current().Name, Version: "1.0.1-beta.3", Build: "59", OS: "linux", Arch: runtime.GOARCH},
		{Name: buildinfo.Current().Name, Version: "1.0.1-beta.3", Build: "59", OS: runtime.GOOS, Arch: "amd64"},
		{Name: "other", Version: "1.0.1-beta.3", Build: "59", OS: runtime.GOOS, Arch: runtime.GOARCH},
	} {
		if err := bundledBackendMatches(info, "1.0.1-beta.3", "59"); err == nil {
			t.Fatalf("accepted mismatched bundled backend %+v", info)
		}
	}
	if err := bundledBackendMatches(matching, "1.0.1-beta.3", ""); err == nil {
		t.Fatal("accepted bundled backend against incomplete App metadata")
	}
}
