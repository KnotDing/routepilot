package dnsserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type PFLease struct {
	Token  string `json:"token"`
	Digest string `json:"digest"`
	PID    int    `json:"pid"`
	path   string
}

func pfCommand(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/sbin/pfctl", args...)
	out, e := cmd.CombinedOutput()
	if e != nil {
		return nil, errors.New("PF operation failed; inspect the private service log and anchor configuration")
	}
	return out, nil
}
func pfDigest(ctx context.Context) (string, error) {
	rules, e := pfCommand(ctx, "-a", "com.apple/surge-controller", "-sr")
	if e != nil {
		return "", e
	}
	nat, e := pfCommand(ctx, "-a", "com.apple/surge-controller", "-sn")
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(append(rules, nat...))
	return hex.EncodeToString(sum[:]), nil
}
func LoadPF(ctx context.Context, c PFConfig, dir string) (*PFLease, error) {
	rules, e := c.Rules()
	if e != nil {
		return nil, e
	}
	if os.Geteuid() != 0 {
		return nil, errors.New("transparent PF management requires the configured administrator service")
	}
	root, e := pfCommand(ctx, "-sr")
	if e != nil || !strings.Contains(string(root), `anchor "com.apple/*"`) {
		return nil, errors.New("root filter anchor does not reach com.apple/*")
	}
	nat, e := pfCommand(ctx, "-sn")
	if e != nil || !strings.Contains(string(nat), `rdr-anchor "com.apple/*"`) {
		return nil, errors.New("root rdr anchor does not reach com.apple/*")
	}
	leasePath := filepath.Join(dir, "pf-lease.json")
	var previous PFLease
	hasPrevious := false
	if b, e := os.ReadFile(leasePath); e == nil {
		if json.Unmarshal(b, &previous) != nil {
			return nil, errors.New("invalid previous PF lease")
		}
		if process, e := os.FindProcess(previous.PID); e == nil && previous.PID != os.Getpid() {
			if e := process.Signal(syscall.Signal(0)); e == nil || errors.Is(e, syscall.EPERM) {
				return nil, errors.New("previous PF owner is still active")
			}
		}
		hasPrevious = true
	}
	filter, e := pfCommand(ctx, "-a", "com.apple/surge-controller", "-sr")
	if e != nil {
		return nil, e
	}
	translations, e := pfCommand(ctx, "-a", "com.apple/surge-controller", "-sn")
	if e != nil {
		return nil, e
	}
	if strings.Contains(string(filter), "block ") || strings.Contains(string(filter), "pass ") || strings.Contains(string(translations), "rdr on") {
		digest, e := pfDigest(ctx)
		if e != nil || !hasPrevious || digest != previous.Digest {
			return nil, errors.New("PF anchor has an unrecognized owner; refusing to replace it")
		}
	}
	file, e := os.CreateTemp(dir, "pf-anchor-*.conf")
	if e != nil {
		return nil, e
	}
	name := file.Name()
	defer os.Remove(name)
	if _, e = file.WriteString(rules); e != nil {
		file.Close()
		return nil, e
	}
	if e = file.Close(); e != nil {
		return nil, e
	}
	if _, e = pfCommand(ctx, "-n", "-a", "com.apple/surge-controller", "-f", name); e != nil {
		return nil, e
	}
	ref, e := pfCommand(ctx, "-E")
	if e != nil {
		return nil, e
	}
	token := ""
	for _, line := range strings.Split(string(ref), "\n") {
		if _, v, ok := strings.Cut(line, "Token : "); ok {
			token = strings.TrimSpace(v)
		}
	}
	if _, e := strconv.ParseUint(token, 10, 64); e != nil {
		return nil, errors.New("PF enable reference token missing")
	}
	l := &PFLease{Token: token, PID: os.Getpid(), path: leasePath}
	if _, e = pfCommand(ctx, "-a", "com.apple/surge-controller", "-f", name); e != nil {
		_, _ = pfCommand(ctx, "-X", token)
		return nil, e
	}
	l.Digest, e = pfDigest(ctx)
	if e != nil {
		_ = l.Close()
		return nil, e
	}
	b, _ := json.Marshal(l)
	if e = os.WriteFile(leasePath, b, 0600); e != nil {
		_ = l.Close()
		return nil, e
	}
	if hasPrevious && previous.Token != "" {
		_, _ = pfCommand(ctx, "-X", previous.Token)
	}
	return l, nil
}
func (l *PFLease) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if l.Digest != "" {
		digest, err := pfDigest(ctx)
		if err != nil || digest != l.Digest {
			return errors.New("PF anchor ownership changed; refusing to flush it")
		}
	}
	_, e := pfCommand(ctx, "-a", "com.apple/surge-controller", "-F", "rules")
	_, ne := pfCommand(ctx, "-a", "com.apple/surge-controller", "-F", "nat")
	_, xe := pfCommand(ctx, "-X", l.Token)
	if e == nil && ne == nil && xe == nil {
		_ = os.Remove(l.path)
		return nil
	}
	return errors.New("could not release own PF anchor/reference")
}

// CleanupPF releases a persisted lease after launchd has stopped the owner.
// It cannot remove an active owner's anchor or an anchor with changed rules.
func CleanupPF(dir string) error {
	if os.Geteuid() != 0 {
		return errors.New("PF cleanup requires administrator privileges")
	}
	path := filepath.Join(dir, "pf-lease.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("invalid private PF lease file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lease PFLease
	if json.Unmarshal(b, &lease) != nil || lease.PID < 1 || lease.Digest == "" {
		return errors.New("invalid persisted PF owner")
	}
	if _, err := strconv.ParseUint(lease.Token, 10, 64); err != nil {
		return errors.New("invalid PF reference token")
	}
	process, err := os.FindProcess(lease.PID)
	if err == nil {
		if err := process.Signal(syscall.Signal(0)); err == nil || errors.Is(err, syscall.EPERM) {
			return errors.New("PF owner is still active")
		}
	}
	lease.path = path
	return lease.Close()
}
