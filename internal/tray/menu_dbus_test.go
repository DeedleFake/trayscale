package tray

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

func TestAccountMenuPlacement(t *testing.T) {
	addr := startMenuBus(t)

	var initial Tray
	if err := initial.Start(twoProfiles("b")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { initial.Close() })

	layout := hostLayout(t, addr, itemBusName(t))
	assertAccountPlacement(t, layout, "al__ice (Tailnet)", "bob")

	initial.Close()

	var late Tray
	if err := late.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { late.Close() })
	late.SetProfiles(twoProfiles("b"))
	late.SetActiveProfile("b")

	layout = hostLayout(t, addr, itemBusName(t))
	assertAccountPlacement(t, layout, "al__ice (Tailnet)", "bob")
}

func TestUnchangedProfileStatusEmitsNothing(t *testing.T) {
	addr := startMenuBus(t)

	var tr Tray
	status := twoProfiles("b")
	if err := tr.Start(status); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { tr.Close() })

	layout, props := menuSignalsDuring(t, addr, func() {
		tr.SetProfiles(status)
		tr.SetActiveProfile(status.Profile.ID)
		tr.SetProfiles(reversedProfiles(status))
		tr.SetActiveProfile(status.Profile.ID)
	})
	if layout != 0 || props != 0 {
		t.Fatalf("unchanged profile status emitted LayoutUpdated=%d ItemsPropertiesUpdated=%d, want 0 and 0", layout, props)
	}

	changedLayout, changedProps := menuSignalsDuring(t, addr, func() {
		tr.SetActiveProfile("a")
	})
	if changedLayout == 0 && changedProps == 0 {
		t.Fatal("real active-profile change produced no dbusmenu signal")
	}
	root := hostLayout(t, addr, itemBusName(t))
	account := accountEntry(t, root)
	if toggleState(account.children[0]) != "1" || toggleState(account.children[1]) != "0" {
		t.Fatalf("after activating a, children = %#v", account.children)
	}
}

func assertAccountPlacement(t *testing.T, root menuNode, childLabels ...string) {
	t.Helper()
	want := []string{
		"Show",
		sepLabel,
		"Connect",
		"Enable exit node",
		"This machine: Not connected",
		"Switch account",
		sepLabel,
		"Quit",
	}
	if got := root.labels(); !equalStrings(got, want) {
		t.Fatalf("root labels = %q\nraw:\n%s", got, root.raw)
	}
	account := accountEntry(t, root)
	if got := labelsOf(account.children); !equalStrings(got, childLabels) {
		t.Fatalf("account children = %q, want %q\nraw:\n%s", got, childLabels, root.raw)
	}
	if toggleState(account.children[0]) != "0" || toggleState(account.children[1]) != "1" {
		t.Fatalf("toggle states = %q %q, want 0 1\nraw:\n%s", toggleState(account.children[0]), toggleState(account.children[1]), root.raw)
	}
}

func accountEntry(t *testing.T, root menuNode) menuNode {
	t.Helper()
	for _, child := range root.children {
		if child.label == "Switch account" {
			return child
		}
	}
	t.Fatalf("Switch account missing from %q\nraw:\n%s", root.labels(), root.raw)
	return menuNode{}
}

func twoProfiles(active ipn.ProfileID) *tsutil.ProfileStatus {
	profiles := []ipn.LoginProfile{
		{
			ID:   "a",
			Name: "al_ice",
			NetworkProfile: ipn.NetworkProfile{
				DomainName:  "tn.ts.net",
				DisplayName: "Tailnet",
			},
		},
		{ID: "b", Name: "bob"},
	}
	var current ipn.LoginProfile
	for _, profile := range profiles {
		if profile.ID == active {
			current = profile
		}
	}
	return &tsutil.ProfileStatus{Profile: current, Profiles: profiles}
}

func reversedProfiles(ps *tsutil.ProfileStatus) *tsutil.ProfileStatus {
	profiles := append([]ipn.LoginProfile(nil), ps.Profiles...)
	for i, j := 0, len(profiles)-1; i < j; i, j = i+1, j-1 {
		profiles[i], profiles[j] = profiles[j], profiles[i]
	}
	return &tsutil.ProfileStatus{Profile: ps.Profile, Profiles: profiles}
}

const sepLabel = "/"

type menuNode struct {
	label    string
	props    string
	children []menuNode
	raw      string
}

func (n menuNode) labels() []string {
	return labelsOf(n.children)
}

func labelsOf(nodes []menuNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.label
	}
	return out
}

func toggleState(n menuNode) string {
	const key = "'toggle-state': <"
	i := strings.Index(n.props, key)
	if i < 0 {
		return ""
	}
	rest := n.props[i+len(key):]
	rest = strings.TrimPrefix(rest, "int32 ")
	end := strings.IndexByte(rest, '>')
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func hostLayout(t *testing.T, addr, dest string) menuNode {
	t.Helper()
	cmd := exec.Command("gdbus", "call", "-e",
		"-d", dest,
		"-o", "/StatusNotifierMenu",
		"-m", "com.canonical.dbusmenu.GetLayout",
		"int32 0", "int32 -1", "@as []",
	)
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+addr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GetLayout: %v\n%s", err, out)
	}
	raw := strings.TrimSpace(string(out))
	node, err := parseLayout(raw)
	if err != nil {
		t.Fatalf("parse layout: %v\n%s", err, raw)
	}
	node.raw = raw
	return node
}

func parseLayout(raw string) (menuNode, error) {
	s := strings.TrimPrefix(strings.TrimSpace(raw), "(")
	s = strings.TrimSuffix(s, ")")
	_, rest, ok := splitTop(s, 1)
	if !ok {
		return menuNode{}, fmt.Errorf("revision split")
	}
	return parseNode(strings.TrimSpace(rest))
}

func parseNode(s string) (menuNode, error) {
	s = unwrapNode(strings.TrimSpace(s))
	parts, ok := splitAllTop(s)
	if !ok || len(parts) < 3 {
		return menuNode{}, fmt.Errorf("node %q", s)
	}
	props := parts[1]
	label := propString(props, "label")
	if propString(props, "type") == "separator" {
		label = sepLabel
	}
	children, err := parseChildren(parts[2])
	if err != nil {
		return menuNode{}, err
	}
	return menuNode{label: label, props: props, children: children}, nil
}

func parseChildren(s string) ([]menuNode, error) {
	s = strings.TrimSpace(s)
	if s == "@av []" || s == "[]" || s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "@av ") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "@av"))
	}
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("children %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil, nil
	}
	parts, ok := splitAllTop(inner)
	if !ok {
		return nil, fmt.Errorf("child list %q", inner)
	}
	nodes := make([]menuNode, 0, len(parts))
	for _, part := range parts {
		node, err := parseNode(part)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func unwrapNode(s string) string {
	for {
		switch {
		case strings.HasPrefix(s, "<(") && strings.HasSuffix(s, ")>"):
			s = strings.TrimSpace(s[2 : len(s)-2])
		case strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"):
			inner := strings.TrimSpace(s[1 : len(s)-1])
			if !balanced(inner) {
				return s
			}
			s = inner
		default:
			return s
		}
	}
}

func propString(props, key string) string {
	token := "'" + key + "': <'"
	i := strings.Index(props, token)
	if i < 0 {
		return ""
	}
	rest := props[i+len(token):]
	end := strings.IndexByte(rest, '\'')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func splitTop(s string, n int) (head, tail string, ok bool) {
	parts, ok := splitAllTop(s)
	if !ok || len(parts) <= n {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:], ", "), true
}

func splitAllTop(s string) ([]string, bool) {
	var parts []string
	depth := 0
	inStr := false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == '\'' {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'':
			inStr = true
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth == 0 {
				return nil, false
			}
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if inStr || depth != 0 {
		return nil, false
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts, true
}

func balanced(s string) bool {
	_, ok := splitAllTop(s)
	return ok
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func menuSignalsDuring(t *testing.T, addr string, fn func()) (layoutUpdated, propsUpdated int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "dbus-monitor", "--session")
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+addr)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("dbus-monitor: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		return bytes.Contains(buf.Bytes(), []byte("NameAcquired"))
	})
	fn()
	time.Sleep(200 * time.Millisecond)
	cancel()
	_ = cmd.Wait()
	text := buf.String()
	if layoutUpdated, propsUpdated = strings.Count(text, "member=LayoutUpdated"), strings.Count(text, "member=ItemsPropertiesUpdated"); layoutUpdated != 0 || propsUpdated != 0 {
		t.Logf("dbus-monitor:\n%s", text)
	}
	return layoutUpdated, propsUpdated
}

func startMenuBus(t *testing.T) string {
	t.Helper()
	for _, bin := range []string{"dbus-daemon", "gdbus", "dbus-monitor"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " is not installed")
		}
	}

	dir := t.TempDir()
	sock := filepath.Join(dir, "bus")
	bus := exec.Command("dbus-daemon", "--session", "--nofork", "--nopidfile",
		"--address=unix:path="+sock, "--print-address=1")
	bus.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := bus.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	bus.Stderr = &bytes.Buffer{}
	if err := bus.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopGroup(bus) })
	reader := bufio.NewReader(out)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("bus address: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	addr := strings.TrimSpace(line)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)

	logPath := filepath.Join(dir, "watcher.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	watcher := exec.Command(watcherBin(t))
	watcher.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	watcher.Stdout = logFile
	watcher.Stderr = logFile
	if err := watcher.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopGroup(watcher)
		logFile.Close()
	})
	waitFor(t, 5*time.Second, func() bool {
		b, _ := os.ReadFile(logPath)
		return bytes.Contains(b, []byte("watcher ready"))
	})
	t.Setenv("TRAY_WATCHER_LOG", logPath)
	return addr
}

func itemBusName(t *testing.T) string {
	t.Helper()
	logPath := os.Getenv("TRAY_WATCHER_LOG")
	var name string
	waitFor(t, 5*time.Second, func() bool {
		b, err := os.ReadFile(logPath)
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(b), "\n") {
			const key = `service="`
			i := strings.LastIndex(line, key)
			if i < 0 || !strings.Contains(line, "RegisterStatusNotifierItem") {
				continue
			}
			rest := line[i+len(key):]
			end := strings.IndexByte(rest, '"')
			if end > 0 {
				name = rest[:end]
			}
		}
		return name != ""
	})
	return name
}

func watcherBin(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "deedles.dev/tray")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("locate tray module: %v", err)
	}
	dir := strings.TrimSpace(string(out))
	bin := filepath.Join(t.TempDir(), "watcher")
	build := exec.Command("go", "build", "-o", bin, "./.cursor/skills/verify-tray/scripts/watcher")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build watcher: %v\n%s", err, out)
	}
	return bin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func waitFor(t *testing.T, d time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}

func stopGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}
