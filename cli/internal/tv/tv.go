package tv

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
	"unicode"

	"github.com/jack-work/aguatv/cli/internal/adb"
)

const (
	KeyUp          = 19
	KeyDown        = 20
	KeyLeft        = 21
	KeyRight       = 22
	KeyCenter      = 23
	KeyBack        = 4
	KeyHome        = 3
	KeyMenu        = 82
	KeyPlayPause   = 85
	KeyRewind      = 89
	KeyFastForward = 90
	KeyVolumeUp    = 24
	KeyVolumeDown  = 25
	KeyMute        = 164
	KeyPower       = 26
	KeyDel         = 67
	KeyEnter       = 66
	KeyWakeup      = 224
	KeySleep       = 223
)

const (
	actionDown = 0
	actionUp   = 1
)

const remoteJar = "/data/local/tmp/aguatv-remote.jar"

type TV interface {
	Press(code int) error
	Shell(args ...string) error
	Hold(code int, d time.Duration) error
	Text(s string) error
	Name() string
	Close()
}

type scrcpy struct {
	a       adb.Device
	cmd     *exec.Cmd
	conn    net.Conn
	forward string
}

func openScrcpy(a adb.Device, jar, version string) (*scrcpy, error) {
	if _, err := a.Run("push", jar, remoteJar); err != nil {
		return nil, err
	}
	scid := fmt.Sprintf("%08x", rand.Uint32()&0x7fffffff)
	socket := "localabstract:scrcpy_" + scid
	port, err := a.Run("forward", "tcp:0", socket)
	if err != nil {
		return nil, err
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("adb forward gave %q", port)
	}
	s := &scrcpy{a: a, forward: "tcp:" + port}
	s.cmd = exec.Command(a.Bin, a.Args("shell",
		"CLASSPATH="+remoteJar, "app_process", "/", "com.genymobile.scrcpy.Server", version,
		"scid="+scid, "log_level=warn", "video=false", "audio=false", "control=true",
		"tunnel_forward=true", "send_device_meta=false", "send_dummy_byte=true",
		"clipboard_autosync=false", "cleanup=false")...)
	stderr, _ := s.cmd.StderrPipe()
	stdout, _ := s.cmd.StdoutPipe()
	if err := s.cmd.Start(); err != nil {
		s.Close()
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
			s.Close()
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
	s.Close()
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

func (s *scrcpy) Press(code int) error {
	if err := s.key(actionDown, code, 0); err != nil {
		return err
	}
	return s.key(actionUp, code, 0)
}

func (s *scrcpy) Hold(code int, d time.Duration) error {
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

func (s *scrcpy) Text(t string) error {
	b := make([]byte, 5+len(t))
	b[0] = 1
	binary.BigEndian.PutUint32(b[1:], uint32(len(t)))
	copy(b[5:], t)
	_, err := s.conn.Write(b)
	return err
}

func (s *scrcpy) Shell(args ...string) error {
	_, err := s.a.Shell(args...)
	return err
}

func (s *scrcpy) Name() string { return "scrcpy" }

func (s *scrcpy) Close() {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.forward != "" {
		s.a.Run("forward", "--remove", s.forward)
	}
}

type shellInput struct{ a adb.Device }

func (s shellInput) Press(code int) error {
	_, err := s.a.Run("shell", "input", "keyevent", strconv.Itoa(code))
	return err
}

func (s shellInput) Hold(code int, _ time.Duration) error {
	_, err := s.a.Run("shell", "input", "keyevent", "--longpress", strconv.Itoa(code))
	return err
}

func (s shellInput) Text(t string) error {
	arg, err := InputTextArg(t)
	if err != nil {
		return err
	}
	_, err = s.a.Run("shell", "input text "+arg)
	return err
}

var ErrUnsafeText = errors.New("text may hold only printable characters")

func InputTextArg(t string) (string, error) {
	for _, r := range t {
		if r < 0x20 || r == 0x7f || !unicode.IsPrint(r) {
			return "", ErrUnsafeText
		}
	}
	return ShellQuote(strings.ReplaceAll(t, " ", "%s")), nil
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s shellInput) Shell(args ...string) error {
	_, err := s.a.Shell(args...)
	return err
}

func (s shellInput) Name() string { return "adb input (slow)" }

func (s shellInput) Close() {}

func Open(d adb.Device, jar, version string, slow bool) (TV, string) {
	if slow || jar == "" || version == "" {
		return shellInput{d}, ""
	}
	s, err := openScrcpy(d, jar, version)
	if err != nil {
		return shellInput{d}, "fell back to adb input: " + err.Error()
	}
	return s, ""
}
