package main

import (
	"os"

	"github.com/jack-work/aguatv/cli/internal/play"

	tea "github.com/charmbracelet/bubbletea"
)

type presignedMsg struct{ a act }

func playRef(ref string) tea.Cmd {
	return func() tea.Msg {
		link, err := play.Resolve(ref, os.Getenv("AGUATV_PRESIGN"))
		if err != nil {
			return sendErrMsg{err}
		}
		title := play.Title(ref)
		return presignedMsg{act{shell: []string{play.Command(link, title, play.Type(ref))}, label: "play " + title, lit: "play"}}
	}
}
