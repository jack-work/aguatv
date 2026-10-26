package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jack-work/aguatv/cli/internal/adb"
)

var defaultAdb = "adb"

const homePkg = "tv.aguatv"
const guardLabel = "aguatv home button"

func readDevice(d adb.Device, c Config) (Device, error) {
	dev := Device{
		Known:     map[string]bool{},
		Installed: map[string]bool{},
		Disabled:  map[string]bool{},
		Starved:   map[string]bool{},
		Settings:  map[string]string{},
	}
	lists := []struct {
		flag string
		into map[string]bool
	}{{"-u", dev.Known}, {"", dev.Installed}, {"-d", dev.Disabled}}
	for _, l := range lists {
		args := []string{"pm", "list", "packages"}
		if l.flag != "" {
			args = append(args, l.flag)
		}
		out, err := d.Shell(args...)
		if err != nil {
			return dev, err
		}
		for _, line := range strings.Split(out, "\n") {
			if name, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok {
				l.into[name] = true
			}
		}
	}

	var script strings.Builder
	var keys []string
	for ns, kv := range c.Settings {
		for key := range kv {
			keys = append(keys, settingKey(ns, key))
		}
	}
	mem, _ := loadMemory(c.Name)
	for k := range mem.OrigSettings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ns, key, _ := strings.Cut(k, "/")
		fmt.Fprintf(&script, "echo \"@@%s\"; settings get %s %s; ", k, shellQuote(ns), shellQuote(key))
	}
	var starveCheck []string
	for name, p := range c.Packages {
		if dev.Known[name] && (p.Protected || p.State == "enabled") {
			starveCheck = append(starveCheck, name)
		}
	}
	for _, name := range mem.Starved {
		starveCheck = append(starveCheck, name)
	}
	for _, name := range starveCheck {
		fmt.Fprintf(&script, "echo \"@@appops/%s\"; cmd appops get %s RUN_ANY_IN_BACKGROUND; ", name, name)
	}
	if script.Len() > 0 {
		out, err := d.Shell(script.String())
		if err != nil {
			return dev, err
		}
		var cur string
		for _, line := range strings.Split(out, "\n") {
			if k, ok := strings.CutPrefix(line, "@@"); ok {
				cur = k
				continue
			}
			if name, ok := strings.CutPrefix(cur, "appops/"); ok {
				if strings.Contains(line, "ignore") {
					dev.Starved[name] = true
				}
				continue
			}
			if cur != "" {
				dev.Settings[cur] = strings.TrimSpace(line)
				cur = ""
			}
		}
	}

	acc, err := d.Shell("dumpsys", "accessibility")
	if err != nil {
		return dev, err
	}
	dev.HomeOK = dev.Installed[homePkg] && strings.Contains(acc, "Service[label="+guardLabel+",")
	return dev, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func memoryPath(name string) string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "aguatv", name+".json")
}

func loadMemory(name string) (Memory, error) {
	var m Memory
	b, err := os.ReadFile(memoryPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

func saveMemory(name string, m Memory) error {
	p := memoryPath(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func apply(d adb.Device, a Action) (Action, error) {
	switch a.Kind {
	case Disable:
		out, err := d.Shell("pm", "disable-user", "--user", "0", a.Pkg)
		if strings.Contains(out, "protected package") {
			a.Kind = Starve
			a.Why = "Fire OS protects it, so it was starved instead; mark it protected"
			return apply(d, a)
		}
		if err == nil && !strings.Contains(out, "disabled-user") {
			err = errors.New(out)
		}
		return a, err
	case Starve, Unstarve:
		mode := "ignore"
		if a.Kind == Unstarve {
			mode = "default"
		}
		for _, op := range []string{"RUN_IN_BACKGROUND", "RUN_ANY_IN_BACKGROUND"} {
			if _, err := d.Shell("cmd", "appops", "set", a.Pkg, op, mode); err != nil {
				return a, err
			}
		}
		return a, nil
	case Enable:
		_, err := d.Shell("pm", "enable", a.Pkg)
		return a, err
	case Reinstall:
		out, err := d.Shell("cmd", "package", "install-existing", a.Pkg)
		if err == nil && strings.Contains(out, "Failure") {
			err = errors.New(out)
		}
		return a, err
	case Uninstall:
		out, err := d.Shell("pm", "uninstall", "--user", "0", a.Pkg)
		if err == nil && !strings.Contains(out, "Success") {
			err = errors.New(out)
		}
		return a, err
	case Put:
		_, err := d.Shell("settings", "put", a.NS, shellQuote(a.Key), shellQuote(a.Value))
		return a, err
	case Restore:
		if a.Value == "null" || a.Value == "" {
			_, err := d.Shell("settings", "delete", a.NS, shellQuote(a.Key))
			return a, err
		}
		_, err := d.Shell("settings", "put", a.NS, shellQuote(a.Key), shellQuote(a.Value))
		return a, err
	case InstallHome, UndoHome:
		cli := os.Getenv("AGUATV_CLI")
		if cli == "" {
			return a, errors.New("AGUATV_CLI is not set; run deploy through the flake")
		}
		verb := "install"
		if a.Kind == UndoHome {
			verb = "undo"
		}
		cmd := exec.Command(cli, verb)
		cmd.Env = append(os.Environ(), "ANDROID_SERIAL="+d.Serial)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return a, cmd.Run()
	}
	return a, fmt.Errorf("unknown action %d", a.Kind)
}

func confirm(prompt string) bool {
	fmt.Print(prompt + " [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")
}

func interactive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func offerRemote(noRemote bool) {
	pkg := os.Getenv("AGUATV_REMOTE_PKG")
	if noRemote || pkg == "" || !interactive() {
		return
	}
	if _, err := exec.LookPath("aguatv-remote"); err == nil {
		return
	}
	if !confirm("aguatv-remote is not on your PATH. Install it into your nix profile?") {
		fmt.Println("skipped; --no-remote stops this question")
		return
	}
	cmd := exec.Command("nix", "profile", "install", pkg)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nix profile install failed:", err)
	}
}

func main() {
	var (
		yes, dry, noRemote bool
		adbBin, serial     string
	)
	flag.BoolVar(&yes, "yes", false, "apply without asking")
	flag.BoolVar(&dry, "dry-run", false, "print the plan and change nothing")
	flag.BoolVar(&noRemote, "no-remote", false, "do not offer to install aguatv-remote")
	flag.StringVar(&adbBin, "adb", defaultAdb, "adb binary")
	flag.StringVar(&serial, "s", "", "adb serial, overriding the configured address")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: aguatv-deploy [-dry-run] [-yes] [-no-remote] <tv.json>\n\nMakes a TV match its configuration. Normally run as `nix run .#<tv>` from a flake using aguatv.lib.mkTvs.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), adbBin, serial, yes, dry, noRemote); err != nil {
		fmt.Fprintln(os.Stderr, "aguatv-deploy:", err)
		os.Exit(1)
	}
}

func run(path, adbBin, serial string, yes, dry, noRemote bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if serial == "" && c.Serial != nil {
		serial = *c.Serial
	}
	addr := c.Address
	if serial != "" {
		addr = ""
	}
	d, err := adb.Open(adbBin, serial, addr)
	if err != nil {
		return err
	}
	dev, err := readDevice(d, c)
	if err != nil {
		return err
	}
	prev, err := loadMemory(c.Name)
	if err != nil {
		return err
	}
	apk := os.Getenv("AGUATV_APK")
	actions, skipped := Plan(c, dev, prev, apk)

	fmt.Printf("%s (%s, %s)\n", c.Name, d.Label(), d.Serial)
	if len(skipped) > 0 {
		fmt.Printf("  %d configured packages are not on this TV\n", len(skipped))
	}
	for _, name := range Drift(c, dev, prev) {
		fmt.Printf("  note: %s is disabled on the TV but not in the config\n", name)
	}
	if len(actions) == 0 {
		fmt.Println("  nothing to do")
		if !dry {
			if err := saveMemory(c.Name, Remember(prev, c, dev, nil, nil, apk)); err != nil {
				return err
			}
		}
		offerRemote(noRemote)
		return nil
	}
	for _, a := range actions {
		fmt.Println("  " + a.String())
	}
	if dry {
		return nil
	}
	if !yes && !confirm(fmt.Sprintf("apply %d changes?", len(actions))) {
		return errors.New("cancelled")
	}
	var done []Action
	var failed int
	var failedPkgs []string
	for _, a := range actions {
		got, err := apply(d, a)
		if err != nil {
			failed++
			failedPkgs = append(failedPkgs, a.Pkg)
			fmt.Printf("  FAILED %s: %v\n", a, err)
			continue
		}
		if got.Kind != a.Kind {
			fmt.Printf("  %s\n", got)
		}
		done = append(done, got)
	}
	if err := saveMemory(c.Name, Remember(prev, c, dev, done, failedPkgs, apk)); err != nil {
		return err
	}
	fmt.Printf("  applied %d, failed %d\n", len(done), failed)
	offerRemote(noRemote)
	if failed > 0 {
		return fmt.Errorf("%d changes failed", failed)
	}
	return nil
}
