package main

import (
	"github.com/jack-work/aguatv/cli/internal/tv"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type recorder struct{ sent []act }

func (r *recorder) send(a act) bool {
	r.sent = append(r.sent, a)
	return true
}

func (r *recorder) codes() []int {
	var out []int
	for _, a := range r.sent {
		if a.text != "" {
			out = append(out, -1)
			continue
		}
		out = append(out, a.code)
	}
	return out
}

func keys(t *testing.T, m model, in ...tea.KeyMsg) model {
	t.Helper()
	var tm tea.Model = m
	for _, k := range in {
		tm, _ = tm.Update(k)
	}
	return tm.(model)
}

func runes(s string) []tea.KeyMsg {
	var out []tea.KeyMsg
	for _, r := range s {
		out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return out
}

func fresh() (model, *recorder) {
	r := &recorder{}
	return newModel(r.send, nil), r
}

func TestHjklDriveTheDpad(t *testing.T) {
	m, r := fresh()
	keys(t, m, runes("hjkl")...)
	if got, want := r.codes(), []int{tv.KeyLeft, tv.KeyDown, tv.KeyUp, tv.KeyRight}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCountRepeatsTheNextKeyOnly(t *testing.T) {
	m, r := fresh()
	m = keys(t, m, runes("3jl")...)
	if got, want := r.codes(), []int{tv.KeyDown, tv.KeyDown, tv.KeyDown, tv.KeyRight}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if m.count != "" {
		t.Fatalf("count left behind: %q", m.count)
	}
}

func TestLoneZeroIsNotACount(t *testing.T) {
	m, r := fresh()
	m = keys(t, m, runes("0j")...)
	if got := r.codes(); !reflect.DeepEqual(got, []int{tv.KeyDown}) {
		t.Fatalf("got %v", got)
	}
}

func TestEscCancelsACountBeforeItMeansBack(t *testing.T) {
	m, r := fresh()
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	m = keys(t, m, append(runes("4"), esc)...)
	if len(r.sent) != 0 || m.count != "" {
		t.Fatalf("esc should only clear the count, sent %v count %q", r.codes(), m.count)
	}
	keys(t, m, esc)
	if got := r.codes(); !reflect.DeepEqual(got, []int{tv.KeyBack}) {
		t.Fatalf("got %v", got)
	}
}

func TestGPrefix(t *testing.T) {
	m, r := fresh()
	keys(t, m, runes("ggxgsgh")...)
	got := r.sent
	if len(got) != 3 || got[0].code != tv.KeyHome || !reflect.DeepEqual(got[1].shell, aguatvSettings.shell) || got[2].code != tv.KeyHome || got[2].hold == 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestInsertModeTypesAndEscLeaves(t *testing.T) {
	m, r := fresh()
	m = keys(t, m, runes("ihj")...)
	m = keys(t, m, tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != normal {
		t.Fatalf("still in mode %d", m.mode)
	}
	if len(r.sent) != 3 || r.sent[0].text != "h" || r.sent[1].text != "j" || r.sent[2].code != tv.KeyDel {
		t.Fatalf("got %+v", r.sent)
	}
	keys(t, m, runes("j")...)
	if last := r.sent[len(r.sent)-1]; last.code != tv.KeyDown {
		t.Fatalf("j after esc should be down, got %+v", last)
	}
}

func TestCommandLine(t *testing.T) {
	m, r := fresh()
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	m = keys(t, m, runes(":power")...)
	m = keys(t, m, enter)
	if m.mode != normal || len(r.sent) != 1 || r.sent[0].code != tv.KeyPower {
		t.Fatalf("mode %d sent %+v", m.mode, r.sent)
	}
	m = keys(t, m, runes(":type hi there")...)
	keys(t, m, enter)
	if r.sent[1].text != "hi there" {
		t.Fatalf("got %+v", r.sent[1])
	}
}

func TestQuitFromEveryMode(t *testing.T) {
	for _, prefix := range []string{"", "i", ":"} {
		m, _ := fresh()
		m = keys(t, m, runes(prefix)...)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatalf("ctrl+c after %q returned no command", prefix)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("ctrl+c after %q did not quit", prefix)
		}
	}
}

func TestPressLightsAndUnlights(t *testing.T) {
	m, _ := fresh()
	tm, cmd := m.Update(runes("k")[0])
	m = tm.(model)
	if m.lit != "up" || cmd == nil {
		t.Fatalf("lit %q", m.lit)
	}
	tm, _ = m.Update(unlightMsg{m.litSeq})
	if tm.(model).lit != "" {
		t.Fatal("did not unlight")
	}
}

func TestDetailsAndBothSettings(t *testing.T) {
	m, r := fresh()
	keys(t, m, runes("KgsgS")...)
	if len(r.sent) != 3 || r.sent[0].code != tv.KeyMenu {
		t.Fatalf("got %+v", r.sent)
	}
	if !reflect.DeepEqual(r.sent[1].shell[len(r.sent[1].shell)-1], "settings") {
		t.Fatalf("gs should open aguatv's settings, got %v", r.sent[1].shell)
	}
	if r.sent[2].shell[len(r.sent[2].shell)-1] != "android.intent.category.LEANBACK_SETTINGS" {
		t.Fatalf("gS should open the TV's settings, got %v", r.sent[2].shell)
	}
}

func TestPlayResolvesOffTheEventLoop(t *testing.T) {
	m, r := fresh()
	m = keys(t, m, runes(":play https://example.com/a.mp4")...)
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || len(r.sent) != 0 {
		t.Fatalf("expected a command and nothing sent yet, sent %+v", r.sent)
	}
	msg, ok := cmd().(presignedMsg)
	if !ok {
		t.Fatalf("got %T", cmd())
	}
	tm.Update(msg)
	if len(r.sent) != 1 || !strings.Contains(r.sent[0].shell[0], "-d 'https://example.com/a.mp4'") {
		t.Fatalf("got %+v", r.sent)
	}
}
