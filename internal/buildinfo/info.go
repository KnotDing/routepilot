// Package buildinfo exposes the release identity shared with the web interface.
package buildinfo

import (
	_ "embed"
	"encoding/json"
	"runtime"
)

//go:embed release.json
var release []byte

type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Build     string `json:"build"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

var identity = func() Info {
	var info Info
	if err := json.Unmarshal(release, &info); err != nil {
		panic(err)
	}
	info.OS, info.Arch, info.GoVersion = runtime.GOOS, runtime.GOARCH, runtime.Version()
	return info
}()

func Current() Info { return identity }
