// Package launchd installs the wallpaper as a per-user LaunchAgent.
package launchd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const Label = "com.towiron.towascii"

// Runner executes a command and returns its combined output; tests replace it.
type Runner func(name string, args ...string) ([]byte, error)

func ExecRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Agent struct {
	Program   string
	LogPath   string
	PlistPath string
	UID       int
	Run       Runner
}

func NewAgent(program string) (Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Agent{}, err
	}
	return Agent{
		Program:   program,
		LogPath:   filepath.Join(home, "Library", "Logs", "towascii.log"),
		PlistPath: filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		UID:       os.Getuid(),
		Run:       ExecRunner,
	}, nil
}

// KeepAlive with SuccessfulExit=false restarts the agent after a crash but
// not after the user picks Quit from the menu, which exits with status 0.
var plistTemplate = template.Must(template.New("plist").Funcs(template.FuncMap{"xml": xmlEscape}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{xml .Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{xml .Program}}</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>{{xml .LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{xml .LogPath}}</string>
</dict>
</plist>
`))

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (a Agent) Plist() []byte {
	var b bytes.Buffer
	_ = plistTemplate.Execute(&b, struct {
		Label, Program, LogPath string
	}{Label, a.Program, a.LogPath})
	return b.Bytes()
}

func (a Agent) domain() string {
	return fmt.Sprintf("gui/%d", a.UID)
}

// Install writes the plist and (re)loads the agent, so running it twice is safe.
func (a Agent) Install() error {
	if err := os.MkdirAll(filepath.Dir(a.PlistPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.LogPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(a.PlistPath, a.Plist(), 0o644); err != nil {
		return err
	}
	// Not loaded yet is the normal case on first install.
	_, _ = a.Run("launchctl", "bootout", a.domain()+"/"+Label)
	if out, err := a.Run("launchctl", "bootstrap", a.domain(), a.PlistPath); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (a Agent) Uninstall() error {
	_, _ = a.Run("launchctl", "bootout", a.domain()+"/"+Label)
	if err := os.Remove(a.PlistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
