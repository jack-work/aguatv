package main

import (
	"fmt"
	"sort"
	"strings"
)

type Package struct {
	State     string  `json:"state"`
	Protected bool    `json:"protected"`
	Pinned    bool    `json:"pinned"`
	Group     *string `json:"group"`
	Note      string  `json:"note"`
}

type Home struct {
	Enable bool `json:"enable"`
}

type Config struct {
	Name     string                       `json:"name"`
	Address  string                       `json:"address"`
	Serial   *string                      `json:"serial"`
	Packages map[string]Package           `json:"packages"`
	Settings map[string]map[string]string `json:"settings"`
	Home     Home                         `json:"home"`
}

type Device struct {
	Known     map[string]bool
	Installed map[string]bool
	Disabled  map[string]bool
	Starved   map[string]bool
	Settings  map[string]string
	HomeOK    bool
	HomeAPK   string
}

type Memory struct {
	Disabled     []string          `json:"disabled"`
	Absent       []string          `json:"absent"`
	Starved      []string          `json:"starved"`
	OrigSettings map[string]string `json:"origSettings"`
	APK          string            `json:"apk"`
}

type Kind int

const (
	Disable Kind = iota
	Starve
	Enable
	Reinstall
	Unstarve
	Uninstall
	Put
	Restore
	InstallHome
	UndoHome
)

var kindNames = map[Kind]string{
	Disable:     "disable",
	Starve:      "starve",
	Enable:      "enable",
	Reinstall:   "reinstall",
	Unstarve:    "unstarve",
	Uninstall:   "uninstall",
	Put:         "set",
	Restore:     "restore",
	InstallHome: "install home",
	UndoHome:    "undo home",
}

type Action struct {
	Kind  Kind
	Pkg   string
	NS    string
	Key   string
	Value string
	Was   string
	Why   string
}

func (a Action) String() string {
	switch a.Kind {
	case Put, Restore:
		return fmt.Sprintf("%-10s %s %s = %s (was %s)", kindNames[a.Kind], a.NS, a.Key, a.Value, a.Was)
	case InstallHome, UndoHome:
		return fmt.Sprintf("%-10s %s", kindNames[a.Kind], a.Why)
	}
	s := fmt.Sprintf("%-10s %s", kindNames[a.Kind], a.Pkg)
	if a.Why != "" {
		s += "  " + a.Why
	}
	return s
}

func settingKey(ns, key string) string { return ns + "/" + key }

func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func why(p Package) string {
	parts := []string{}
	if p.Group != nil && *p.Group != "" {
		parts = append(parts, "["+*p.Group+"]")
	}
	if p.Note != "" {
		parts = append(parts, p.Note)
	}
	return strings.Join(parts, " ")
}

func Plan(c Config, d Device, prev Memory, apk string) (actions []Action, skipped []string) {
	wantDisabled, wantAbsent, wantStarved := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, name := range sortedKeys(c.Packages) {
		p := c.Packages[name]
		if !d.Known[name] {
			if p.State != "unmanaged" {
				skipped = append(skipped, name)
			}
			continue
		}
		switch p.State {
		case "disabled":
			if p.Protected {
				wantStarved[name] = true
				if !d.Starved[name] {
					actions = append(actions, Action{Kind: Starve, Pkg: name, Why: why(p)})
				}
				continue
			}
			wantDisabled[name] = true
			if !d.Installed[name] {
				actions = append(actions, Action{Kind: Reinstall, Pkg: name, Why: "so it can be disabled"})
			}
			if !d.Disabled[name] || !d.Installed[name] {
				actions = append(actions, Action{Kind: Disable, Pkg: name, Why: why(p)})
			}
		case "absent":
			wantAbsent[name] = true
			if d.Installed[name] {
				actions = append(actions, Action{Kind: Uninstall, Pkg: name, Why: why(p)})
			}
		case "enabled":
			if !d.Installed[name] {
				actions = append(actions, Action{Kind: Reinstall, Pkg: name, Why: why(p)})
			}
			if d.Disabled[name] {
				actions = append(actions, Action{Kind: Enable, Pkg: name, Why: why(p)})
			}
			if d.Starved[name] {
				actions = append(actions, Action{Kind: Unstarve, Pkg: name, Why: why(p)})
			}
		}
	}
	for _, name := range prev.Disabled {
		if !wantDisabled[name] && d.Known[name] && d.Disabled[name] && c.Packages[name].State != "enabled" {
			actions = append(actions, Action{Kind: Enable, Pkg: name, Why: "no longer configured"})
		}
	}
	for _, name := range prev.Absent {
		if !wantAbsent[name] && d.Known[name] && !d.Installed[name] && c.Packages[name].State != "enabled" && c.Packages[name].State != "disabled" {
			actions = append(actions, Action{Kind: Reinstall, Pkg: name, Why: "no longer configured"})
		}
	}
	for _, name := range prev.Starved {
		if !wantStarved[name] && d.Known[name] && d.Starved[name] && c.Packages[name].State != "enabled" {
			actions = append(actions, Action{Kind: Unstarve, Pkg: name, Why: "no longer configured"})
		}
	}

	wanted := map[string]bool{}
	for _, ns := range sortedKeys(c.Settings) {
		for _, key := range sortedKeys(c.Settings[ns]) {
			v := c.Settings[ns][key]
			k := settingKey(ns, key)
			wanted[k] = true
			if cur, ok := d.Settings[k]; !ok || cur != v {
				actions = append(actions, Action{Kind: Put, NS: ns, Key: key, Value: v, Was: d.Settings[k]})
			}
		}
	}
	for _, k := range sortedKeys(prev.OrigSettings) {
		if wanted[k] {
			continue
		}
		ns, key, _ := strings.Cut(k, "/")
		actions = append(actions, Action{Kind: Restore, NS: ns, Key: key, Value: prev.OrigSettings[k], Was: d.Settings[k]})
	}

	switch {
	case c.Home.Enable && (!d.HomeOK || prev.APK != apk):
		actions = append(actions, Action{Kind: InstallHome, Why: "aguatv takes over HOME"})
	case !c.Home.Enable && d.HomeOK:
		actions = append(actions, Action{Kind: UndoHome, Why: "HOME goes back to Amazon's launcher"})
	}
	return actions, skipped
}

func Remember(prev Memory, c Config, d Device, done []Action, failed []string, apk string) Memory {
	disabled, absent, starved := set(prev.Disabled), set(prev.Absent), set(prev.Starved)
	orig := map[string]string{}
	for k, v := range prev.OrigSettings {
		orig[k] = v
	}
	for _, a := range done {
		switch a.Kind {
		case Disable:
			disabled[a.Pkg] = true
		case Enable:
			delete(disabled, a.Pkg)
		case Uninstall:
			absent[a.Pkg] = true
		case Reinstall:
			delete(absent, a.Pkg)
		case Starve:
			starved[a.Pkg] = true
		case Unstarve:
			delete(starved, a.Pkg)
		case Put:
			k := settingKey(a.NS, a.Key)
			if _, ok := orig[k]; !ok {
				orig[k] = a.Was
			}
		case Restore:
			delete(orig, settingKey(a.NS, a.Key))
		case InstallHome:
			prev.APK = apk
		case UndoHome:
			prev.APK = ""
		}
	}
	failedPkg := set(failed)
	for name, p := range c.Packages {
		if !d.Known[name] || failedPkg[name] {
			continue
		}
		switch {
		case p.State == "disabled" && p.Protected:
			starved[name] = true
		case p.State == "disabled":
			disabled[name] = true
		case p.State == "absent":
			absent[name] = true
		}
	}
	return Memory{
		Disabled:     sortedKeys(disabled),
		Absent:       sortedKeys(absent),
		Starved:      sortedKeys(starved),
		OrigSettings: orig,
		APK:          prev.APK,
	}
}

func Drift(c Config, d Device, prev Memory) []string {
	remembered := set(prev.Disabled)
	var out []string
	for _, name := range sortedKeys(d.Disabled) {
		p, ok := c.Packages[name]
		if (!ok || p.State == "unmanaged") && !remembered[name] {
			out = append(out, name)
		}
	}
	return out
}
