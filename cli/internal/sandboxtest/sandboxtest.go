package sandboxtest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Sh(t *testing.T) []string {
	t.Helper()
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("needs bwrap to run sh in a sandbox")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("needs sh")
	}
	sh, err = filepath.EvalSymlinks(sh)
	if err != nil || !strings.HasPrefix(sh, "/nix/store/") {
		t.Skip("needs sh from /nix/store, the only tree the sandbox binds")
	}
	argv := []string{
		bwrap, "--unshare-all", "--die-with-parent", "--new-session", "--clearenv",
		"--ro-bind", "/nix/store", "/nix/store",
		"--tmpfs", "/tmp", "--dev", "/dev", "--proc", "/proc",
		"--setenv", "PATH", "/nonexistent",
		"--chdir", "/tmp",
		sh,
	}
	if err := exec.Command(argv[0], append(argv[1:], "-c", "true")...).Run(); err != nil {
		t.Skipf("bwrap cannot start a sandbox here: %v", err)
	}
	return argv
}
