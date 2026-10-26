package main

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const playerComponent = "tv.aguatv.player/.Player"

type presignedMsg struct{ a act }

var videoTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".ts":   "video/mp2t",
}

func refPath(ref string) string {
	if u, err := url.Parse(ref); err == nil && u.Scheme != "" {
		return u.Path
	}
	return ref
}

func videoType(ref string) string {
	if t, ok := videoTypes[strings.ToLower(path.Ext(refPath(ref)))]; ok {
		return t
	}
	return "video/*"
}

func videoTitle(ref string) string {
	return path.Base(refPath(ref))
}

func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func playCommand(link, title, mime string) string {
	return "pm path tv.aguatv.player >/dev/null || { echo 'the player is not on the TV: run aguatv play once'; exit 3; }; " +
		"input keyevent KEYCODE_WAKEUP; " +
		"am start -a android.intent.action.VIEW -n " + playerComponent +
		" -t " + singleQuote(mime) + " -d " + singleQuote(link) + " --es title " + singleQuote(title) + " >/dev/null"
}

func resolve(ref, presign string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("play what? :play <https URL, or a reference AGUATV_PRESIGN understands>")
	}
	if strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	if presign == "" {
		return "", errors.New("not an https URL, and AGUATV_PRESIGN is not set to turn it into one")
	}
	out, err := exec.Command("sh", "-c", presign+` "$1"`, "sh", ref).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", errors.New("presign: " + strings.TrimSpace(string(ee.Stderr)))
		}
		return "", errors.New("presign: " + err.Error())
	}
	link := strings.TrimSpace(string(out))
	if !strings.HasPrefix(link, "https://") || strings.ContainsAny(link, "\n ") {
		return "", errors.New("presign did not print one https URL")
	}
	return link, nil
}

func play(ref string) tea.Cmd {
	return func() tea.Msg {
		link, err := resolve(ref, os.Getenv("AGUATV_PRESIGN"))
		if err != nil {
			return sendErrMsg{err}
		}
		title := videoTitle(ref)
		return presignedMsg{act{shell: []string{playCommand(link, title, videoType(ref))}, label: "play " + title, lit: "play"}}
	}
}
