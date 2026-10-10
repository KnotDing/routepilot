// Package appautomation manages integrations on the Mac running the backend.
// The installed App helper runs as the configured desktop user, never as root.
package appautomation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const bundle = "/Applications/RoutePilot.app"

var agents = map[string]bool{"codex": true, "claude": true, "hermes": true, "cursor": true, "copilot": true, "gemini": true, "opencode": true, "windsurf": true, "cline": true, "roo": true, "kilo": true, "antigravity": true, "openclaw": true, "qwen": true, "amp": true, "pi": true}

type Preferences struct {
	CLIEnabled            *bool    `json:"cliEnabled"`
	AutomaticSkillInstall *bool    `json:"automaticSkillInstall"`
	SkillTargets          []string `json:"skillTargets"`
}

type Change struct {
	Preferences Preferences       `json:"preferences"`
	Choices     map[string]string `json:"choices,omitempty"`
}

func (c Change) Validate() error {
	p := c.Preferences
	if p.CLIEnabled == nil || p.AutomaticSkillInstall == nil || p.SkillTargets == nil || *p.AutomaticSkillInstall && len(p.SkillTargets) == 0 {
		return errors.New("complete CLI and skill preferences are required")
	}
	seen := map[string]bool{}
	for _, id := range p.SkillTargets {
		if !agents[id] || seen[id] {
			return errors.New("invalid or duplicate skill target")
		}
		seen[id] = true
	}
	for id, choice := range c.Choices {
		if id != "cli" && !agents[id] || choice != "overwrite" && choice != "skip" {
			return errors.New("invalid CLI or skill conflict choice")
		}
	}
	return nil
}

type Result struct {
	Supported   bool                `json:"supported"`
	Preferences *Preferences        `json:"preferences,omitempty"`
	Status      map[string]any      `json:"status,omitempty"`
	User        string              `json:"user,omitempty"`
	Code        string              `json:"code,omitempty"`
	Error       string              `json:"error,omitempty"`
	Conflicts   []map[string]string `json:"conflicts,omitempty"`
}

type Manager struct {
	standard bool
	gate     chan struct{}
}

func New(databasePath string) *Manager {
	return &Manager{standard: runtime.GOOS == "darwin" && os.Geteuid() == 0 && databasePath == "/var/db/surge-controller/controller.sqlite", gate: make(chan struct{}, 1)}
}

func owned(path string, directory bool, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 && int(stat.Uid) != uid || info.Mode().Perm()&0022 != 0 || directory != info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("installed App ownership or permissions are unsafe")
	}
	return nil
}

type limitedOutput struct{ buffer bytes.Buffer }

func (b *limitedOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *limitedOutput) String() string { return b.buffer.String() }

func (b *limitedOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 1<<20 {
		return 0, errors.New("App integration response is too large")
	}
	return b.buffer.Write(data)
}

// Call accepts no executable, paths, UID or command from the API caller.
func (m *Manager) Call(ctx context.Context, configuredUser string, change *Change) (Result, error) {
	unsupported := Result{Error: "Update the complete App on this backend to manage CLI and skill integrations"}
	if !m.standard {
		return Result{Error: "CLI and skill management requires the installed macOS service"}, nil
	}
	if change != nil {
		if err := change.Validate(); err != nil {
			return Result{}, err
		}
	}
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	default:
		return Result{Supported: true, Code: "automation_busy", Error: "CLI and skill settings are being applied; retry shortly"}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	marker := bundle + "/Contents/Resources/automation-api.json"
	if _, err := os.Lstat(marker); errors.Is(err, os.ErrNotExist) {
		return unsupported, nil
	}
	var target *user.User
	var err error
	if configuredUser != "" {
		target, err = user.Lookup(configuredUser)
	} else if info, e := os.Stat("/dev/console"); e == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			target, err = user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
		}
	}
	if err != nil || target == nil {
		return Result{}, errors.New("backend desktop user is unavailable")
	}
	uid, err := strconv.Atoi(target.Uid)
	if err != nil || uid < 500 || uid > 2147483647 {
		return Result{}, errors.New("backend desktop user is unavailable")
	}
	for _, path := range []string{bundle, bundle + "/Contents", bundle + "/Contents/MacOS", bundle + "/Contents/Resources"} {
		if err := owned(path, true, uid); err != nil {
			return Result{}, err
		}
	}
	for _, path := range []string{marker, bundle + "/Contents/MacOS/RoutePilot"} {
		if err := owned(path, false, uid); err != nil {
			return Result{}, err
		}
	}
	data, err := os.ReadFile(marker)
	var capability struct {
		Version int `json:"version"`
	}
	if err != nil || len(data) > 1024 || json.Unmarshal(data, &capability) != nil || capability.Version != 1 {
		return unsupported, nil
	}
	verify := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", bundle)
	verify.Env = []string{"PATH=/usr/bin:/bin"}
	if verify.Run() != nil {
		return Result{}, errors.New("cannot validate installed App integration helper")
	}
	if exec.CommandContext(ctx, "/bin/launchctl", "print", "gui/"+target.Uid).Run() != nil {
		return Result{}, errors.New("backend desktop user session is unavailable")
	}
	input := map[string]any{"action": "get"}
	if change != nil {
		input = map[string]any{"action": "set", "preferences": change.Preferences, "choices": change.Choices}
	}
	data, err = json.Marshal(input)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, "/bin/launchctl", "asuser", target.Uid, "/usr/bin/sudo", "-n", "-H", "-u", "#"+target.Uid, bundle+"/Contents/MacOS/RoutePilot", "--manage-automation")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + target.HomeDir, "TMPDIR=/private/tmp"}
	// Only these integration roots can enter the unprivileged helper environment.
	// They come from the target GUI session, never from a remote request.
	variables := []string{}
	for _, key := range []string{"CODEX_HOME", "HERMES_HOME", "PI_CODING_AGENT_DIR"} {
		read := exec.CommandContext(ctx, "/bin/launchctl", "asuser", target.Uid, "/bin/launchctl", "getenv", key)
		read.Env = []string{"PATH=/usr/bin:/bin"}
		value := &limitedOutput{}
		read.Stdout, read.Stderr = value, io.Discard
		if read.Run() == nil {
			root := strings.TrimSpace(value.String())
			if filepath.IsAbs(root) && len(root) <= 4096 && !strings.ContainsAny(root, "\x00\r\n") {
				variables = append(variables, key+"="+root)
			}
		}
	}
	if len(variables) > 0 {
		arguments := []string{"asuser", target.Uid, "/usr/bin/sudo", "-n", "-H", "-u", "#" + target.Uid, "/usr/bin/env"}
		arguments = append(arguments, variables...)
		arguments = append(arguments, bundle+"/Contents/MacOS/RoutePilot", "--manage-automation")
		cmd.Args = append([]string{cmd.Path}, arguments...)
	}
	cmd.Stdin = bytes.NewReader(data)
	output := &limitedOutput{}
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = output, io.Discard, time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, errors.New("App integration helper failed")
	}
	var result Result
	if json.Unmarshal(output.Bytes(), &result) != nil {
		return Result{}, errors.New("invalid App integration response")
	}
	if result.Error == "" && (result.Preferences == nil || result.Status == nil || (Change{Preferences: *result.Preferences}).Validate() != nil) {
		return Result{}, errors.New("incomplete App integration response")
	}
	result.User = target.Username
	return result, nil
}
