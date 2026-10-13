package integration

import (
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type releaseStep struct{ Name, ID, Run, If string }
type releaseJob struct {
	ContinueOnError bool `yaml:"continue-on-error"`
	Needs, If       string
	Outputs         map[string]string
	Steps           []releaseStep
}

// Exercise the actual workflow shell with a local curl fixture: no release
// token, external request, runner configuration or deployed service is used.
func TestReleaseWorkflowMirrorAndFallbackPublication(t *testing.T) {
	raw, err := os.ReadFile("../../.gitea/workflows/release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct{ Jobs map[string]releaseJob }
	if err = yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	mirror, backup := workflow.Jobs["fetch-github-assets"], workflow.Jobs["fallback-build"]
	if !mirror.ContinueOnError || backup.ContinueOnError || backup.Needs != "fetch-github-assets" || !strings.Contains(backup.If, "always()") || !strings.Contains(backup.If, "outputs.published != 'true'") || mirror.Outputs["published"] != "${{ steps.publish.outputs.published }}" {
		t.Fatal("workflow cannot recover a preferred-path failure while retaining a fallback failure")
	}
	find := func(job releaseJob, name string) string {
		for _, s := range job.Steps {
			if s.ID == name || s.Name == name {
				return s.Run
			}
		}
		t.Fatalf("missing workflow step: %s", name)
		return ""
	}
	tests := []struct {
		name, fetch, mirrorPublish, fallbackPublish string
		wantFallback, wantSuccess                   bool
	}{
		{"mirror success", "success", "success", "failure", false, true},
		{"fetch unavailable fallback success", "missing", "success", "success", true, true},
		{"bad checksum fallback success", "corrupt", "success", "success", true, true},
		{"mirror publication failed fallback success", "success", "failure", "success", true, true},
		{"both paths failed", "missing", "failure", "failure", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			// Successful hashes cover exactly the two fixture package files.
			sums := fmt.Sprintf("%x  RoutePilot-macos-arm64.dmg\n%x  RoutePilot-macos-arm64.zip\n", sha256.Sum256([]byte("fixture dmg")), sha256.Sum256([]byte("fixture zip")))
			curl := `#!/usr/bin/env python3
import json,os,pathlib,sys
args=sys.argv[1:]
if '-o' in args:
 p=pathlib.Path(args[args.index('-o')+1]);p.write_text(os.environ['FIXTURE_SUMS'] if p.name=='SHA256SUMS' else 'invalid' if os.environ['FETCH_RESULT']=='corrupt' else 'fixture dmg' if p.suffix=='.dmg' else 'fixture zip');sys.exit(0)
if any('/releases/tags/' in a for a in args):
 if os.environ['FETCH_RESULT']=='missing':sys.exit(22)
 print(json.dumps({'tag_name':'v1.0.4-beta.19','assets':[{'name':n,'browser_download_url':'https://fixture.test/'+n} for n in ['RoutePilot-macos-arm64.dmg','RoutePilot-macos-arm64.zip','SHA256SUMS']]}));sys.exit(0)
if os.environ['PUBLISH_RESULT']=='failure':sys.exit(22)
print(json.dumps({'id':42}))
`
			for name, body := range map[string]string{"curl": curl, "sleep": "#!/bin/sh\nexit 0\n", "make": "#!/bin/sh\nmkdir -p artifacts\nprintf 'fixture dmg' > artifacts/RoutePilot-macos-arm64.dmg\nprintf 'fixture zip' > artifacts/RoutePilot-macos-arm64.zip\n"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			output := filepath.Join(root, "outputs")
			execute := func(script, publish string) error {
				command := exec.Command("bash", "-c", script)
				command.Dir = root
				command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "GITHUB_REF_NAME=v1.0.4-beta.19", "GITHUB_RUN_NUMBER=1", "GITHUB_REPOSITORY=knot/routepilot", "GITHUB_API_URL=https://fixture.test/api/v1", "GITHUB_OUTPUT="+output, "GITEA_TOKEN=fixture-value-not-a-credential", "RELEASE_SOURCE_TOKEN=", "FETCH_RESULT="+tc.fetch, "PUBLISH_RESULT="+publish, "FIXTURE_SUMS="+sums)
				data, err := command.CombinedOutput()
				if err != nil {
					t.Logf("expected candidate-path failure: %s", data)
				}
				return err
			}
			if err := execute(find(mirror, "fetch"), tc.mirrorPublish); err != nil {
				t.Fatal("fetch should finish with an explicit available flag", err)
			}
			data, _ := os.ReadFile(output)
			mirrored := false
			if strings.Contains(string(data), "available=true") {
				err := execute(find(mirror, "publish"), tc.mirrorPublish)
				data, _ = os.ReadFile(output)
				mirrored = err == nil && strings.Contains(string(data), "published=true")
			}
			fallback := !mirrored
			if fallback != tc.wantFallback {
				t.Fatal("incorrect fallback selection", string(data))
			}
			success := mirrored
			if fallback {
				if err := execute(find(backup, "Build fallback package"), tc.fallbackPublish); err != nil {
					t.Fatal(err)
				}
				if err := execute(find(backup, "Generate fallback checksums"), tc.fallbackPublish); err != nil {
					t.Fatal(err)
				}
				success = execute(find(backup, "Publish fallback-built assets to Gitea"), tc.fallbackPublish) == nil
			}
			if success != tc.wantSuccess {
				t.Fatalf("publication success=%t, want %t", success, tc.wantSuccess)
			}
		})
	}
}
