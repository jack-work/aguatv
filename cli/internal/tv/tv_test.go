package tv

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInputTextArgIsOneLiteralWordForSh(t *testing.T) {
	for _, in := range []string{
		"hello",
		"`reboot`",
		"$(pm list packages)",
		"a; reboot",
		"it's",
		`back\slash "quoted" {braces} [brackets] !bang ?q ~tilde #hash &amp *star |pipe <in >out`,
		"'''",
	} {
		arg, err := InputTextArg(in)
		if err != nil {
			t.Fatalf("%q refused: %v", in, err)
		}
		out, err := exec.Command("sh", "-c", "printf '%s' "+arg).Output()
		if err != nil {
			t.Fatalf("%q: sh failed: %v", in, err)
		}
		want := strings.ReplaceAll(in, " ", "%s")
		if string(out) != want {
			t.Fatalf("%q reached the program as %q, want %q", in, out, want)
		}
	}
}

func TestInputTextArgRefusesControlCharacters(t *testing.T) {
	for _, in := range []string{"a\nreboot", "a\rb", "tab\there", "nul\x00", "del\x7f", "esc\x1b[0m"} {
		if _, err := InputTextArg(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}
