package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	normal mode = iota
	insert
	command
)

type act struct {
	shell []string
	code  int
	text  string
	hold  time.Duration
	label string
	lit   string
}

type connectedMsg struct{ via string }
type failedMsg struct{ err error }
type unlightMsg struct{ seq int }
type sendErrMsg struct{ err error }

type model struct {
	send      func(act) bool
	connect   tea.Cmd
	mode      mode
	count     string
	pendingG  bool
	input     textinput.Model
	via       string
	status    string
	last      string
	typed     string
	lit       string
	litSeq    int
	help      bool
	connected bool
	w, h      int
}

func newModel(send func(act) bool, connect tea.Cmd) model {
	in := textinput.New()
	in.Prompt = ":"
	in.CharLimit = 64
	return model{send: send, connect: connect, input: in, status: "connecting"}
}

func (m model) Init() tea.Cmd { return m.connect }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case connectedMsg:
		m.connected, m.via, m.status = true, msg.via, ""
		return m, nil
	case failedMsg:
		m.status = msg.err.Error()
		return m, nil
	case sendErrMsg:
		m.status = msg.err.Error()
		return m, nil
	case unlightMsg:
		if msg.seq == m.litSeq {
			m.lit = ""
		}
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		switch m.mode {
		case insert:
			return m.insertKey(msg)
		case command:
			return m.commandKey(msg)
		default:
			return m.normalKey(msg)
		}
	}
	return m, nil
}

func (m model) times() int {
	n, err := strconv.Atoi(m.count)
	if err != nil || n < 1 {
		return 1
	}
	if n > 50 {
		return 50
	}
	return n
}

func (m model) fire(a act, n int) (model, tea.Cmd) {
	for i := 0; i < n; i++ {
		if !m.send(a) {
			m.status = "busy, key dropped"
			break
		}
	}
	m.last = a.label
	if n > 1 {
		m.last += " ×" + strconv.Itoa(n)
	}
	m.count = ""
	m.pendingG = false
	if a.lit == "" {
		return m, nil
	}
	m.litSeq++
	m.lit = a.lit
	seq := m.litSeq
	return m, tea.Tick(160*time.Millisecond, func(time.Time) tea.Msg { return unlightMsg{seq} })
}

var (
	aguatvSettings = act{shell: []string{"am", "start", "-n", "tv.aguatv/.Home", "--es", "aguatv.open", "settings"}, label: "aguatv settings", lit: "set"}
	tvSettings     = act{shell: []string{"am", "start", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.LEANBACK_SETTINGS"}, label: "TV settings", lit: "set"}
)

var keyActs = map[string]act{
	"K":         {code: keyMenu, label: "details", lit: "info"},
	"k":         {code: keyUp, label: "up", lit: "up"},
	"up":        {code: keyUp, label: "up", lit: "up"},
	"j":         {code: keyDown, label: "down", lit: "down"},
	"down":      {code: keyDown, label: "down", lit: "down"},
	"h":         {code: keyLeft, label: "left", lit: "left"},
	"left":      {code: keyLeft, label: "left", lit: "left"},
	"l":         {code: keyRight, label: "right", lit: "right"},
	"right":     {code: keyRight, label: "right", lit: "right"},
	"enter":     {code: keyCenter, label: "ok", lit: "ok"},
	" ":         {code: keyCenter, label: "ok", lit: "ok"},
	"o":         {code: keyCenter, label: "ok", lit: "ok"},
	"esc":       {code: keyBack, label: "back", lit: "back"},
	"backspace": {code: keyBack, label: "back", lit: "back"},
	"b":         {code: keyBack, label: "back", lit: "back"},
	"H":         {code: keyHome, label: "home", lit: "home"},
	"m":         {code: keyMenu, label: "menu", lit: "menu"},
	"p":         {code: keyPlayPause, label: "play/pause", lit: "play"},
	"[":         {code: keyRewind, label: "rewind", lit: "rew"},
	"]":         {code: keyFastForward, label: "fast forward", lit: "ff"},
	"+":         {code: keyVolumeUp, label: "volume up", lit: "vol+"},
	"=":         {code: keyVolumeUp, label: "volume up", lit: "vol+"},
	"-":         {code: keyVolumeDown, label: "volume down", lit: "vol-"},
	"M":         {code: keyMute, label: "mute", lit: "mute"},
}

func (m model) normalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.pendingG {
		m.pendingG = false
		switch k {
		case "g":
			return m.fire(act{code: keyHome, label: "home", lit: "home"}, 1)
		case "s":
			return m.fire(aguatvSettings, 1)
		case "S":
			return m.fire(tvSettings, 1)
		case "h":
			return m.fire(act{code: keyHome, hold: 1200 * time.Millisecond, label: "quick menu", lit: "home"}, 1)
		}
		m.count = ""
		return m, nil
	}
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && (k != "0" || m.count != "") {
		if len(m.count) < 2 {
			m.count += k
		}
		return m, nil
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.count != "" {
			m.count = ""
			return m, nil
		}
	case "g":
		m.pendingG = true
		return m, nil
	case "i", "a":
		m.mode, m.count, m.typed = insert, "", ""
		return m, nil
	case ":":
		m.mode, m.count = command, ""
		m.input.Reset()
		return m, m.input.Focus()
	case "?":
		m.help = !m.help
		m.count = ""
		return m, nil
	case "ctrl+u":
		return m.fire(act{code: keyUp, label: "up", lit: "up"}, 5*m.times())
	case "ctrl+d":
		return m.fire(act{code: keyDown, label: "down", lit: "down"}, 5*m.times())
	}
	if a, ok := keyActs[k]; ok {
		return m.fire(a, m.times())
	}
	m.count = ""
	return m, nil
}

func (m model) insertKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = normal
		return m, nil
	case tea.KeyBackspace:
		if r := []rune(m.typed); len(r) > 0 {
			m.typed = string(r[:len(r)-1])
		}
		return m.fire(act{code: keyDel, label: "delete"}, 1)
	case tea.KeyEnter:
		m.typed = ""
		return m.fire(act{code: keyEnter, label: "enter", lit: "ok"}, 1)
	case tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight:
		return m.fire(keyActs[msg.String()], 1)
	case tea.KeyRunes, tea.KeySpace:
		s := string(msg.Runes)
		if msg.Type == tea.KeySpace {
			s = " "
		}
		m.typed += s
		if r := []rune(m.typed); len(r) > 24 {
			m.typed = string(r[len(r)-24:])
		}
		return m.fire(act{text: s, label: "type"}, 1)
	}
	return m, nil
}

func (m model) commandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = normal
		m.input.Blur()
		return m, nil
	case tea.KeyEnter:
		line := strings.TrimSpace(m.input.Value())
		m.mode = normal
		m.input.Blur()
		return m.run(line)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

var commands = map[string]act{
	"home":     {code: keyHome, label: "home", lit: "home"},
	"back":     {code: keyBack, label: "back", lit: "back"},
	"menu":     {code: keyMenu, label: "menu", lit: "menu"},
	"settings": aguatvSettings,
	"tv":       tvSettings,
	"details":  {code: keyMenu, label: "details", lit: "info"},
	"power":    {code: keyPower, label: "power", lit: "power"},
	"wake":     {code: keyWakeup, label: "wake", lit: "power"},
	"sleep":    {code: keySleep, label: "sleep", lit: "power"},
	"mute":     {code: keyMute, label: "mute", lit: "mute"},
	"quick":    {code: keyHome, hold: 1200 * time.Millisecond, label: "quick menu", lit: "home"},
}

func (m model) run(line string) (tea.Model, tea.Cmd) {
	switch line {
	case "":
		return m, nil
	case "q", "q!", "quit", "wq", "x":
		return m, tea.Quit
	case "h", "help":
		m.help = true
		return m, nil
	}
	if strings.HasPrefix(line, "type ") {
		return m.fire(act{text: strings.TrimPrefix(line, "type "), label: "type"}, 1)
	}
	if a, ok := commands[line]; ok {
		return m.fire(a, 1)
	}
	m.status = "unknown command: " + line
	return m, nil
}

var (
	gold    = lipgloss.Color("#D9A94E")
	ink     = lipgloss.Color("#F0DFC8")
	muted   = lipgloss.Color("#A5887B")
	oxblood = lipgloss.Color("#7D2E2E")
	stage   = lipgloss.Color("#1A0D0F")

	body   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(oxblood).Padding(1, 2)
	title  = lipgloss.NewStyle().Foreground(gold).Bold(true)
	btn    = lipgloss.NewStyle().Foreground(ink)
	btnLit = lipgloss.NewStyle().Foreground(stage).Background(gold).Bold(true)
	hintSt = lipgloss.NewStyle().Foreground(muted)
	modeSt = lipgloss.NewStyle().Foreground(stage).Background(gold).Bold(true).Padding(0, 1)
	insSt  = lipgloss.NewStyle().Foreground(stage).Background(lipgloss.Color("#9CC47A")).Bold(true).Padding(0, 1)
	cmdSt  = lipgloss.NewStyle().Foreground(ink).Background(oxblood).Bold(true).Padding(0, 1)
	errSt  = lipgloss.NewStyle().Foreground(lipgloss.Color("#EC7A64"))
)

func (m model) button(id, face string, width int) string {
	st := btn
	if m.lit == id {
		st = btnLit
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, st.Render(face))
}

func (m model) remote() string {
	const w = 26
	row := func(cells ...string) string { return lipgloss.JoinHorizontal(lipgloss.Top, cells...) }
	third := w / 3
	dpad := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.PlaceHorizontal(w, lipgloss.Center, m.button("up", " ▲ ", 5)),
		row(m.button("left", " ◀ ", third), m.button("ok", " OK ", w-2*third), m.button("right", " ▶ ", third)),
		lipgloss.PlaceHorizontal(w, lipgloss.Center, m.button("down", " ▼ ", 5)),
	)
	nav := row(m.button("back", " back ", third), m.button("home", " home ", w-2*third), m.button("menu", " menu ", third))
	media := row(m.button("rew", " ◀◀ ", third), m.button("play", " ▶‖ ", w-2*third), m.button("ff", " ▶▶ ", third))
	extra := row(m.button("info", " info ", third), lipgloss.NewStyle().Width(w-2*third).Render(""), m.button("set", " set ", third))
	vol := row(m.button("vol-", " vol- ", third), m.button("mute", " mute ", w-2*third), m.button("vol+", " vol+ ", third))
	head := row(title.Render("aguatv"), lipgloss.PlaceHorizontal(w-6, lipgloss.Right, m.button("power", " ⏻ ", 3)))
	return body.Render(lipgloss.JoinVertical(lipgloss.Left, head, "", dpad, "", nav, extra, "", media, vol))
}

func (m model) statusLine() string {
	var tag string
	switch m.mode {
	case insert:
		tag = insSt.Render("INSERT")
	case command:
		return cmdSt.Render("COMMAND") + " " + m.input.View()
	default:
		tag = modeSt.Render("NORMAL")
	}
	parts := []string{tag}
	if m.count != "" {
		parts = append(parts, m.count)
	}
	if m.pendingG {
		parts = append(parts, "g")
	}
	if m.mode == insert {
		parts = append(parts, hintSt.Render("typing: ")+m.typed+"▏")
	} else if m.last != "" {
		parts = append(parts, hintSt.Render(m.last))
	}
	switch {
	case m.status != "" && !m.connected && m.status == "connecting":
		parts = append(parts, hintSt.Render("connecting…"))
	case m.status != "":
		parts = append(parts, errSt.Render(m.status))
	case m.via != "":
		parts = append(parts, hintSt.Render("· "+m.via))
	}
	return strings.Join(parts, " ")
}

const helpText = `h j k l   d-pad        enter o ␣   ok
b ⌫ esc   back         gg H        home
K m       details      gh :quick   quick menu
p         play/pause   [ ]         rewind / ff
+ = -     volume       M           mute
5j        repeat       ^u ^d       up / down ×5
i a       type text    esc         leave typing
gs        aguatv set.  gS          TV settings
:power    :wake        :sleep      :details
:type txt send text    q :q        quit`

func (m model) View() string {
	view := m.remote()
	if m.help {
		view = lipgloss.JoinVertical(lipgloss.Left, view, hintSt.Render(helpText))
	} else {
		view = lipgloss.JoinVertical(lipgloss.Left, view, hintSt.Render("? for keys"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, view, m.statusLine())
}
