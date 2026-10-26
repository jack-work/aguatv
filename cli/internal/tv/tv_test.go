package tv

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/jack-work/aguatv/cli/internal/sandboxtest"
)

const witness = "INJECTED"

func sandboxedSh(t *testing.T) func(script string) string {
	t.Helper()
	argv := sandboxtest.Sh(t)
	return func(script string) string {
		out, err := exec.Command(argv[0], append(argv[1:], "-c", script)...).CombinedOutput()
		if err != nil {
			t.Fatalf("sandboxed sh: %v: %s", err, out)
		}
		return string(out)
	}
}

func TestInputTextArgIsOneLiteralWordForSh(t *testing.T) {
	run := sandboxedSh(t)
	for _, in := range []string{
		"hello",
		"`: > /tmp/injected`",
		"$(: > /tmp/injected)",
		"a; : > /tmp/injected",
		"it' ; : > /tmp/injected ; '",
		`back\slash "quoted" {braces} [brackets] !bang ?q ~tilde #hash &amp *star |pipe <in >out`,
		"'''",
	} {
		arg, err := InputTextArg(in)
		if err != nil {
			t.Fatalf("%q refused: %v", in, err)
		}
		out := run("printf '%s' " + arg + "; if [ -e /tmp/injected ]; then printf '\\n" + witness + "'; fi")
		if strings.Contains(out, witness) {
			t.Fatalf("%q ran as shell: %q", in, out)
		}
		if want := strings.ReplaceAll(in, " ", "%s"); out != want {
			t.Fatalf("%q reached the program as %q, want %q", in, out, want)
		}
	}
}

func TestInputTextArgRefusesControlCharacters(t *testing.T) {
	for _, in := range []string{"a\n: > /tmp/injected", "a\rb", "tab\there", "nul\x00", "del\x7f", "esc\x1b[0m"} {
		if _, err := InputTextArg(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}
