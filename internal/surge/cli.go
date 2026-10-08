package surge

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

// Runner is replaceable for tests. Production never invokes a shell.
type Runner interface {
	Run(context.Context, ...string) ([]byte, error)
}
type CLI struct {
	Path     string
	Timeout  time.Duration
	UserName string
}

const outputLimit = 4 << 20

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("Surge CLI output limit exceeded")
	}
	return b.Buffer.Write(p)
}
func NewCLI(path string, timeout time.Duration) (*CLI, error) {
	if !filepath.IsAbs(path) || timeout <= 0 {
		return nil, errors.New("absolute Surge CLI path and positive timeout required")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() || st.Mode().Perm()&0111 == 0 {
		return nil, errors.New("Surge CLI is not executable")
	}
	return &CLI{Path: path, Timeout: timeout}, nil
}
func (c *CLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, append([]string{"--raw"}, args...)...)
	var target *user.User
	if runtime.GOOS == "darwin" && os.Geteuid() == 0 {
		var err error
		if c.UserName != "" {
			target, err = user.Lookup(c.UserName)
		} else if info, e := os.Stat("/dev/console"); e == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 {
				target, err = user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
			}
		}
		if err != nil || target == nil || target.Uid == "0" {
			return nil, errors.New("Surge GUI user is unavailable")
		}
		// Enter the GUI user's bootstrap namespace and identity without a shell.
		// Root remains responsible for PF/DNS; only the local CLI drops privilege.
		argv := []string{"asuser", target.Uid, "/usr/bin/sudo", "-n", "-u", "#" + target.Uid, c.Path, "--raw"}
		argv = append(argv, args...)
		cmd = exec.CommandContext(ctx, "/bin/launchctl", argv...)
	}
	// Do not inherit remote passwords, proxy variables, or arbitrary dynamic loader
	// settings. This adapter connects only to the local running instance.
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "HOME=") || strings.HasPrefix(v, "TMPDIR=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	if target != nil {
		cmd.Env = []string{"HOME=" + target.HomeDir, "TMPDIR=/private/tmp"}
	}
	stdout := &cappedBuffer{limit: outputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Surge CLI command failed")
	}
	if !json.Valid(stdout.Bytes()) {
		return nil, errors.New("Surge CLI did not return structured JSON")
	}
	var env struct {
		Error string `json:"error"`
		More  bool   `json:"hasMore"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, err
	}
	if env.Error != "" {
		return nil, errors.New("Surge rejected CLI query")
	}
	if env.More {
		return nil, errors.New("unexpected streaming CLI response")
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}
