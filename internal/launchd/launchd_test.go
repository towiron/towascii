package launchd

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

type recorder struct {
	calls [][]string
	fail  string
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.fail != "" && args[0] == r.fail {
		return []byte("boom"), errors.New("exit status 5")
	}
	return nil, nil
}

func testAgent(t *testing.T, rec *recorder) Agent {
	dir := t.TempDir()
	return Agent{
		Program:   "/Users/me/.local/bin/ascii-wallpaper",
		LogPath:   filepath.Join(dir, "Logs", "ascii-wallpaper.log"),
		PlistPath: filepath.Join(dir, "LaunchAgents", Label+".plist"),
		UID:       501,
		Run:       rec.run,
	}
}

func TestPlistGolden(t *testing.T) {
	a := Agent{Program: "/Users/me/bin/ascii & wallpaper", LogPath: "/Users/me/Library/Logs/ascii-wallpaper.log"}
	got := a.Plist()
	path := filepath.Join("testdata", "agent.plist")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("plist changed:\n%s", got)
	}
	if !strings.Contains(string(got), "ascii &amp; wallpaper") {
		t.Error("program path not XML-escaped")
	}
}

func TestInstall(t *testing.T) {
	rec := &recorder{}
	a := testAgent(t, rec)
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.PlistPath); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"launchctl", "bootout", "gui/501/" + Label},
		{"launchctl", "bootstrap", "gui/501", a.PlistPath},
	}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Errorf("calls = %v, want %v", rec.calls, want)
	}
}

func TestInstallReportsBootstrapFailure(t *testing.T) {
	rec := &recorder{fail: "bootstrap"}
	err := testAgent(t, rec).Install()
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want launchctl output in it", err)
	}
}

func TestUninstall(t *testing.T) {
	rec := &recorder{}
	a := testAgent(t, rec)
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	rec.calls = nil
	if err := a.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.PlistPath); !os.IsNotExist(err) {
		t.Errorf("plist still there: %v", err)
	}
	if err := a.Uninstall(); err != nil {
		t.Errorf("second uninstall: %v", err)
	}
	if len(rec.calls) != 2 || rec.calls[0][1] != "bootout" {
		t.Errorf("calls = %v", rec.calls)
	}
}
