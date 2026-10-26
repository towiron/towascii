package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/towiron/towascii/internal/config"
	"github.com/towiron/towascii/internal/launchd"
	"github.com/towiron/towascii/internal/mac"
	"github.com/towiron/towascii/internal/render"
)

// Cocoa must run on the process's main thread, and only init is guaranteed
// to execute there before the scheduler can move the goroutine.
func init() {
	runtime.LockOSThread()
}

const usage = `usage: towascii [command]

commands:
  run        show the live wallpaper (default)
  install    start at login via a LaunchAgent
  uninstall  remove the LaunchAgent
  dump       print one frame to the terminal (see dump -h)
`

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "run":
		err = run()
	case "install":
		err = install()
	case "uninstall":
		err = uninstall()
	case "dump":
		err = dump(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "towascii:", err)
		os.Exit(1)
	}
}

type app struct {
	cfg     config.Config
	cfgPath string
	scenes  map[uint32]*render.Scene
	seed    uint64
}

func (a *app) Resize(screen uint32, width, height float64) render.Layout {
	s, ok := a.scenes[screen]
	if !ok {
		s = render.NewScene(rand.New(rand.NewPCG(a.seed, uint64(screen))), a.cfg.White())
		a.scenes[screen] = s
	}
	return s.Resize(width, height)
}

func (a *app) Frame(screen uint32, dt float64) ([]render.Item, bool) {
	s, ok := a.scenes[screen]
	if !ok {
		return nil, false
	}
	return s.Frame(dt), true
}

func (a *app) ScreenRemoved(screen uint32) {
	delete(a.scenes, screen)
}

func (a *app) Action(act mac.Action) {
	switch act {
	case mac.ActionBlack:
		a.cfg.Background = config.Black
	case mac.ActionWhite:
		a.cfg.Background = config.White
	case mac.ActionPause:
		a.cfg.Paused = !a.cfg.Paused
	case mac.ActionQuit:
		mac.Quit()
		return
	}
	if err := config.Save(a.cfgPath, a.cfg); err != nil {
		slog.Warn("save config", "err", err)
	}
	for _, s := range a.scenes {
		s.Renderer.SetWhite(a.cfg.White())
	}
	mac.Apply(a.cfg.White(), a.cfg.Paused)
}

func run() error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		slog.Warn("config ignored, using defaults", "err", err)
	}
	a := &app{cfg: cfg, cfgPath: path, scenes: map[uint32]*render.Scene{}, seed: uint64(time.Now().UnixNano())}
	// launchctl bootout and Ctrl-C send signals; route them through Cocoa's
	// terminate so the user's desktop picture is restored on the way out.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		mac.QuitAsync()
	}()
	mac.Run(a, cfg.White(), cfg.Paused)
	return nil
}

func agent() (launchd.Agent, error) {
	exe, err := os.Executable()
	if err != nil {
		return launchd.Agent{}, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return launchd.Agent{}, err
	}
	return launchd.NewAgent(exe)
}

func install() error {
	a, err := agent()
	if err != nil {
		return err
	}
	if err := a.Install(); err != nil {
		return err
	}
	fmt.Println("installed", a.PlistPath, "->", a.Program)
	return nil
}

func uninstall() error {
	a, err := agent()
	if err != nil {
		return err
	}
	if err := a.Uninstall(); err != nil {
		return err
	}
	// The agent restores the picture itself on SIGTERM; this covers an agent
	// that was not running or had crashed.
	mac.RestoreWallpaper()
	fmt.Println("removed", a.PlistPath)
	return nil
}

func dump(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	w := fs.Float64("width", 1440, "screen width in points")
	h := fs.Float64("height", 900, "screen height in points")
	secs := fs.Float64("t", 8, "seconds to simulate before the frame")
	seed := fs.Uint64("seed", uint64(time.Now().UnixNano()), "random seed")
	white := fs.Bool("white", false, "white background")
	plain := fs.Bool("plain", false, "no colours")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	s := render.NewScene(rand.New(rand.NewPCG(*seed, 0)), *white)
	l := s.Resize(*w, *h)
	var items []render.Item
	for t := 0.0; t < *secs; t += 1.0 / 30 {
		items = s.Frame(1.0 / 30)
	}
	if *plain {
		fmt.Print(render.Text(items, l))
		return nil
	}
	bg := render.RGBA(0x000000FF)
	if *white {
		bg = 0xFFFFFFFF
	}
	fmt.Print(render.ANSI(items, l, bg))
	return nil
}
