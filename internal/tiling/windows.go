package tiling

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"deskctl/internal/fancywm"
	"deskctl/internal/winapi"
)

// WindowsDesktop drives real windows identified by their HWND in decimal.
type WindowsDesktop struct{}

func handle(id string) (uintptr, error) {
	value, err := strconv.ParseUint(id, 10, strconv.IntSize)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("identite de fenetre invalide")
	}
	return uintptr(value), nil
}

func (WindowsDesktop) Rect(id string) (winapi.Rect, error) {
	hwnd, err := handle(id)
	if err != nil {
		return winapi.Rect{}, err
	}
	return winapi.VisibleWindowRect(hwnd)
}

func (WindowsDesktop) SetRect(id string, rect winapi.Rect) error {
	hwnd, err := handle(id)
	if err != nil {
		return err
	}
	return winapi.SetVisibleWindowRect(hwnd, rect)
}

func (WindowsDesktop) Focus(id string) error {
	hwnd, err := handle(id)
	if err != nil {
		return err
	}
	return winapi.ForceForeground(hwnd)
}

func (WindowsDesktop) Wait(duration time.Duration) { time.Sleep(duration) }

// FancyCLI runs FancyWM actions through its command line.
type FancyCLI struct {
	Context    context.Context
	Client     fancywm.Client
	Executable string
}

func (cli FancyCLI) Running() bool {
	name := filepath.Base(cli.Executable)
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	return winapi.ProcessRunning(name) || winapi.ProcessRunning("FancyWM.exe")
}

func (cli FancyCLI) Action(action string) error {
	return cli.Client.Action(cli.Context, cli.Executable, action)
}
