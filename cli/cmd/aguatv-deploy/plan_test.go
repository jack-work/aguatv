package main

import (
	"reflect"
	"testing"
)

func device(known, installed, disabled, starved []string, settings map[string]string) Device {
	if settings == nil {
		settings = map[string]string{}
	}
	return Device{Known: set(known), Installed: set(installed), Disabled: set(disabled), Starved: set(starved), Settings: settings}
}

func kinds(as []Action) []string {
	var out []string
	for _, a := range as {
		out = append(out, kindNames[a.Kind]+" "+a.Pkg+a.Key)
	}
	return out
}

func TestDisableOnlyWhatIsEnabledAndPresent(t *testing.T) {
	c := Config{Packages: map[string]Package{
		"a":    {State: "disabled"},
		"b":    {State: "disabled"},
		"gone": {State: "disabled"},
	}}
	d := device([]string{"a", "b"}, []string{"a", "b"}, []string{"b"}, nil, nil)
	acts, skipped := Plan(c, d, Memory{}, "", "")
	if got := kinds(acts); !reflect.DeepEqual(got, []string{"disable a"}) {
		t.Fatalf("got %v", got)
	}
	if !reflect.DeepEqual(skipped, []string{"gone"}) {
		t.Fatalf("skipped %v", skipped)
	}
}

func TestProtectedIsStarvedNotDisabled(t *testing.T) {
	c := Config{Packages: map[string]Package{"p": {State: "disabled", Protected: true}}}
	d := device([]string{"p"}, []string{"p"}, nil, nil, nil)
	acts, _ := Plan(c, d, Memory{}, "", "")
	if got := kinds(acts); !reflect.DeepEqual(got, []string{"starve p"}) {
		t.Fatalf("got %v", got)
	}
	d.Starved["p"] = true
	if acts, _ := Plan(c, d, Memory{}, "", ""); len(acts) != 0 {
		t.Fatalf("already starved, got %v", kinds(acts))
	}
}

func TestPinnedEnabledRepairsADisabledPackage(t *testing.T) {
	c := Config{Packages: map[string]Package{"v": {State: "enabled", Pinned: true}}}
	d := device([]string{"v"}, []string{"v"}, []string{"v"}, nil, nil)
	acts, _ := Plan(c, d, Memory{}, "", "")
	if got := kinds(acts); !reflect.DeepEqual(got, []string{"enable v"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDroppingAPackageFromTheConfigUndoesIt(t *testing.T) {
	c := Config{Packages: map[string]Package{}}
	d := device([]string{"a", "s", "u"}, []string{"a", "s"}, []string{"a"}, []string{"s"}, nil)
	prev := Memory{Disabled: []string{"a"}, Starved: []string{"s"}, Absent: []string{"u"}}
	acts, _ := Plan(c, d, prev, "", "")
	if got := kinds(acts); !reflect.DeepEqual(got, []string{"enable a", "reinstall u", "unstarve s"}) {
		t.Fatalf("got %v", got)
	}
}

func TestHandDisabledPackagesAreDriftNotUndone(t *testing.T) {
	c := Config{Packages: map[string]Package{}}
	d := device([]string{"x"}, []string{"x"}, []string{"x"}, nil, nil)
	if acts, _ := Plan(c, d, Memory{}, "", ""); len(acts) != 0 {
		t.Fatalf("got %v", kinds(acts))
	}
	if got := Drift(c, d, Memory{}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("drift %v", got)
	}
}

func TestSettingsAreSetThenRestoredWhenDropped(t *testing.T) {
	c := Config{Settings: map[string]map[string]string{"secure": {"sleep_timeout": "-1"}}}
	d := device(nil, nil, nil, nil, map[string]string{"secure/sleep_timeout": "840000"})
	acts, _ := Plan(c, d, Memory{}, "", "")
	if len(acts) != 1 || acts[0].Kind != Put || acts[0].Was != "840000" {
		t.Fatalf("got %+v", acts)
	}
	mem := Remember(Memory{}, c, d, acts, nil, "", "")
	if mem.OrigSettings["secure/sleep_timeout"] != "840000" {
		t.Fatalf("memory %+v", mem)
	}
	d.Settings["secure/sleep_timeout"] = "-1"
	acts, _ = Plan(Config{}, d, mem, "", "")
	if len(acts) != 1 || acts[0].Kind != Restore || acts[0].Value != "840000" {
		t.Fatalf("got %+v", acts)
	}
}

func TestHomeReinstallsWhenTheApkChanges(t *testing.T) {
	c := Config{Home: Home{Enable: true}}
	d := device(nil, nil, nil, nil, nil)
	d.HomeOK = true
	if acts, _ := Plan(c, d, Memory{APK: "/nix/store/a"}, "/nix/store/a", ""); len(acts) != 0 {
		t.Fatalf("got %v", kinds(acts))
	}
	if acts, _ := Plan(c, d, Memory{APK: "/nix/store/a"}, "/nix/store/b", ""); len(acts) != 1 || acts[0].Kind != InstallHome {
		t.Fatalf("got %v", kinds(acts))
	}
}

func TestRememberAdoptsWhatAlreadyHolds(t *testing.T) {
	c := Config{Packages: map[string]Package{
		"a": {State: "disabled"},
		"p": {State: "disabled", Protected: true},
		"f": {State: "disabled"},
		"x": {State: "disabled"},
	}}
	d := device([]string{"a", "p", "f"}, []string{"a", "p", "f"}, []string{"a"}, []string{"p"}, nil)
	mem := Remember(Memory{}, c, d, nil, []string{"f"}, "", "")
	if !reflect.DeepEqual(mem.Disabled, []string{"a"}) || !reflect.DeepEqual(mem.Starved, []string{"p"}) {
		t.Fatalf("memory %+v", mem)
	}
}

func TestPlayerIsInstalledWhenAskedAndRemovedOnlyIfDeployPutItThere(t *testing.T) {
	on := Config{Player: Player{Enable: true}}
	d := device([]string{playerPkg}, nil, nil, nil, nil)
	if acts, _ := Plan(on, d, Memory{}, "", "/p1"); len(acts) != 1 || acts[0].Kind != InstallPlayer {
		t.Fatalf("got %v", kinds(acts))
	}
	d.Installed[playerPkg] = true
	if acts, _ := Plan(on, d, Memory{PlayerAPK: "/p1"}, "", "/p1"); len(acts) != 0 {
		t.Fatalf("current player, got %v", kinds(acts))
	}
	off := Config{}
	if acts, _ := Plan(off, d, Memory{}, "", "/p1"); len(acts) != 0 {
		t.Fatalf("a player deploy did not install is left alone, got %v", kinds(acts))
	}
	if acts, _ := Plan(off, d, Memory{PlayerAPK: "/p1"}, "", "/p1"); len(acts) != 1 || acts[0].Kind != RemovePlayer {
		t.Fatalf("got %v", kinds(acts))
	}
}
