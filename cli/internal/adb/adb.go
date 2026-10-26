package adb

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Device struct {
	Bin    string
	Serial string
}

func (d Device) Args(rest ...string) []string {
	if d.Serial == "" {
		return rest
	}
	return append([]string{"-s", d.Serial}, rest...)
}

func (d Device) Command(rest ...string) *exec.Cmd {
	return exec.Command(d.Bin, d.Args(rest...)...)
}

func (d Device) Run(rest ...string) (string, error) {
	out, err := d.Command(rest...).CombinedOutput()
	s := strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	if err != nil {
		return s, fmt.Errorf("adb %s: %v: %s", strings.Join(rest, " "), err, s)
	}
	return s, nil
}

func (d Device) Shell(cmd ...string) (string, error) {
	return d.Run(append([]string{"shell"}, cmd...)...)
}

func (d Device) Connect(addr string) error {
	out, err := d.Run("connect", addr)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "connected") {
		return errors.New(out)
	}
	return nil
}

func (d Device) Ready() error {
	out, err := d.Run("get-state")
	if err != nil {
		return fmt.Errorf("no TV: %s", out)
	}
	if out != "device" {
		return fmt.Errorf("TV is %s; accept the debugging prompt on the TV", out)
	}
	return nil
}

func (d Device) Label() string {
	for _, q := range [][]string{{"settings", "get", "global", "device_name"}, {"getprop", "ro.product.model"}} {
		out, err := d.Shell(q...)
		if err == nil && out != "" && out != "null" {
			return out
		}
	}
	return d.Serial
}

func Open(bin, serial, addr string) (Device, error) {
	d := Device{Bin: bin, Serial: serial}
	if addr != "" {
		if err := d.Connect(addr); err != nil {
			return d, err
		}
		if d.Serial == "" {
			d.Serial = addr
		}
	}
	return d, d.Ready()
}
