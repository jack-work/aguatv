package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	keyUp          = 19
	keyDown        = 20
	keyLeft        = 21
	keyRight       = 22
	keyCenter      = 23
	keyBack        = 4
	keyHome        = 3
	keyMenu        = 82
	keyPlayPause   = 85
	keyRewind      = 89
	keyFastForward = 90
	keyVolumeUp    = 24
	keyVolumeDown  = 25
	keyMute        = 164
	keyPower       = 26
	keyDel         = 67
	keyEnter       = 66
	keySettings    = 176
	keyWakeup      = 224
	keySleep       = 223
)

const (
	actionDown = 0
	actionUp   = 1
)

const remoteJar = "/data/local/tmp/aguatv-remote.jar"

type tv interface {
	press(code int) error
	hold(code int, d time.Duration) error
	text(s string) error
	name() string
	close()
}

type adb struct {
	bin    string
	serial string
}

func (a adb) args(rest ...string) []string {
	if a.serial == "" {
		return rest
	}
	return append([]string{"-s", a.serial}, rest...)
}

func (a adb) run(rest ...string) (string, error) {
	out, err := exec.Command(a.bin, a.args(rest...)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("adb %s: %v: %s", strings.Join(rest, " "), err, s)
	}
	return s, nil
}

func (a adb) connect(addr string) error {
	out, err := a.run("connect", addr)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "connected") {
		return errors.New(out)
	}
	return nil
}

func (a adb) ready() error {
	out, err := a.run("get-state")
	if err != nil {
		return fmt.Errorf("no TV: %s", out)
	}
	if out != "device" {
		return fmt.Errorf("TV is %s; accept the debugging prompt on the TV", out)
	}
	return nil
}

type scrcpy struct {
	a       adb
	cmd     *exec.Cmd
	conn    net.Conn
	forward string
}

func openScrcpy(a adb, jar, version string) (*scrcpy, error) {
	if _, err := a.run("push", jar, remoteJar); err != nil {
		return nil, err
	}
	scid := fmt.Sprintf("%08x", rand.Uint32()&0x7fffffff)
	socket := "localabstract:scrcpy_" + scid
	port, err := a.run("forward", "tcp:0", socket)
	if err != nil {
		return nil, err
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("adb forward gave %q", port)
	}
	s := &scrcpy{a: a, forward: "tcp:" + port}
	s.cmd = exec.Command(a.bin, a.args("shell",
		"CLASSPATH="+remoteJar, "app_process", "/", "com.genymobile.scrcpy.Server", version,
		"scid="+scid, "log_level=warn", "video=false", "audio=false", "control=true",
		"tunnel_forward=true", "send_device_meta=false", "send_dummy_byte=true",
		"clipboard_autosync=false", "cleanup=false")...)
	stderr, _ := s.cmd.StderrPipe()
	stdout, _ := s.cmd.StdoutPipe()
	if err := s.cmd.Start(); err != nil {
		s.close()
		return nil, err
	}
	failed := make(chan string, 1)
	go func() {
		var lines []string
		sc := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for sc.Scan() {
			if len(lines) < 8 {
				lines = append(lines, sc.Text())
			}
		}
		failed <- strings.Join(lines, "; ")
	}()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case msg := <-failed:
			s.close()
			return nil, fmt.Errorf("scrcpy server exited: %s", msg)
		default:
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
		if err == nil {
			conn.SetReadDeadline(time.Now().Add(time.Second))
			b := make([]byte, 1)
			if _, err := io.ReadFull(conn, b); err == nil {
				conn.SetReadDeadline(time.Time{})
				s.conn = conn
				go io.Copy(io.Discard, conn)
				return s, nil
			}
			conn.Close()
		}
		time.Sleep(150 * time.Millisecond)
	}
	s.close()
	return nil, errors.New("scrcpy server did not answer")
}

func (s *scrcpy) key(action, code, repeat int) error {
	b := make([]byte, 14)
	b[0] = 0
	b[1] = byte(action)
	binary.BigEndian.PutUint32(b[2:], uint32(code))
	binary.BigEndian.PutUint32(b[6:], uint32(repeat))
	binary.BigEndian.PutUint32(b[10:], 0)
	_, err := s.conn.Write(b)
	return err
}

func (s *scrcpy) press(code int) error {
	if err := s.key(actionDown, code, 0); err != nil {
		return err
	}
	return s.key(actionUp, code, 0)
}

func (s *scrcpy) hold(code int, d time.Duration) error {
	if err := s.key(actionDown, code, 0); err != nil {
		return err
	}
	step := 50 * time.Millisecond
	for i := 1; time.Duration(i)*step < d; i++ {
		time.Sleep(step)
		if err := s.key(actionDown, code, i); err != nil {
			return err
		}
	}
	return s.key(actionUp, code, 0)
}

func (s *scrcpy) text(t string) error {
	b := make([]byte, 5+len(t))
	b[0] = 1
	binary.BigEndian.PutUint32(b[1:], uint32(len(t)))
	copy(b[5:], t)
	_, err := s.conn.Write(b)
	return err
}

func (s *scrcpy) name() string { return "scrcpy" }

func (s *scrcpy) close() {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.forward != "" {
		s.a.run("forward", "--remove", s.forward)
	}
}

type shellInput struct{ a adb }

func (s shellInput) press(code int) error {
	_, err := s.a.run("shell", "input", "keyevent", strconv.Itoa(code))
	return err
}

func (s shellInput) hold(code int, _ time.Duration) error {
	_, err := s.a.run("shell", "input", "keyevent", "--longpress", strconv.Itoa(code))
	return err
}

func (s shellInput) text(t string) error {
	escaped := strings.NewReplacer(" ", "%s", "'", "\\'", "\"", "\\\"", "&", "\\&", "(", "\\(", ")", "\\)",
		";", "\\;", "<", "\\<", ">", "\\>", "|", "\\|", "*", "\\*", "$", "\\$").Replace(t)
	_, err := s.a.run("shell", "input", "text", escaped)
	return err
}

func (s shellInput) name() string { return "adb input (slow)" }

func (s shellInput) close() {}
