package main

import (
	"fmt"
	"log"
	"os"

	"deskctl/frontend"
	"deskctl/internal/gui"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--check" {
		inventory, err := (windoworganizer.WindowsBackend{}).Inventory()
		if err != nil {
			log.Fatal(err)
		}
		blocked := 0
		for _, window := range inventory.Windows {
			if window.Blocked != "" {
				blocked++
			}
		}
		fmt.Printf("Inventaire : %d bureaux, %d fenetres, %d non deplacables. Aucun mouvement.\n", len(inventory.Desktops), len(inventory.Windows), blocked)
		return
	}
	winapi.EnablePerMonitorDPI()
	app, err := gui.New()
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title: "deskctl — Atelier des bureaux",
		Width: 1440, Height: 920, MinWidth: 960, MinHeight: 640,
		BackgroundColour: options.NewRGB(244, 243, 237),
		AssetServer:      &assetserver.Options{Assets: frontend.Assets},
		OnStartup:        gui.Startup(app), OnBeforeClose: gui.BeforeClose(app),
		OnShutdown:         gui.Shutdown(app),
		Bind:               []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "deskctl-window-atelier"},
	})
	if err != nil {
		log.Fatal(err)
	}
}
