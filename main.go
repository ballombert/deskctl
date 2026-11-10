package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"deskctl/internal/desktopfiles"
	"deskctl/internal/tui"
	"deskctl/internal/vdesktop"
	"deskctl/internal/winapi"
)

func printUsage() {
	fmt.Println(`Usage de deskctl :
  deskctl tui

  deskctl windows export [--out windows.json]
	deskctl windows apply --layout layout.json [--windows windows.json] [--execute]

  deskctl desktop scan [--out desktop-files.json]
  deskctl desktop plan --input desktop-files.json [--rules rules.json] [--out desktop-plan.json]
  deskctl desktop apply --plan desktop-plan.json [--execute] [--undo-log undo-log.json]
  deskctl desktop undo --log undo-log.json [--execute]`)
}

func notifyOneDriveNotice(path string) {
	if strings.Contains(strings.ToLower(path), "onedrive") {
		fmt.Fprintf(os.Stderr, "ℹ️  Information OneDrive : Le Bureau est situé sous OneDrive (%s).\n    Les déplacements de fichiers seront automatiquement répercutés sur le cloud.\n\n", path)
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	category := os.Args[1]

	switch category {
	case "tui":
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur TUI : %v\n", err)
			os.Exit(1)
		}
	case "windows":
		handleWindowsCommand(os.Args[2:])
	case "desktop":
		handleDesktopCommand(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Erreur : commande inconnue %q\n\n", category)
		printUsage()
		os.Exit(1)
	}
}

func handleWindowsCommand(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Erreur : sous-commande windows requise (export, apply)")
		os.Exit(1)
	}

	subcmd := args[0]
	switch subcmd {
	case "export":
		fs := flag.NewFlagSet("windows export", flag.ExitOnError)
		outFile := fs.String("out", "windows.json", "Fichier de sortie JSON")
		fs.Parse(args[1:])

		windows, err := winapi.EnumerateUserWindows()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lors de l'inventaire des fenêtres : %v\n", err)
			os.Exit(1)
		}

		data, err := json.MarshalIndent(windows, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur de sérialisation JSON : %v\n", err)
			os.Exit(1)
		}

		if *outFile == "-" {
			fmt.Println(string(data))
		} else {
			if err := os.WriteFile(*outFile, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur lors de l'écriture de %s : %v\n", *outFile, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stdout, "Succès : %d fenêtre(s) exportée(s) dans %s\n", len(windows), *outFile)
		}

	case "apply":
		fs := flag.NewFlagSet("windows apply", flag.ExitOnError)
		layoutFile := fs.String("layout", "layout.json", "Fichier de disposition layout.json")
		windowsFile := fs.String("windows", "windows.json", "Fichier d'inventaire original windows.json (pour vérifier les PIDs recyclés)")
		execute := fs.Bool("execute", false, "Appliquer réellement les changements (simulation par défaut)")
		fs.Parse(args[1:])

		layoutData, err := os.ReadFile(*layoutFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture layout %s : %v\n", *layoutFile, err)
			os.Exit(1)
		}

		entries, err := vdesktop.ValidateLayout(layoutData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur validation layout : %v\n", err)
			os.Exit(1)
		}

		winData, err := os.ReadFile(*windowsFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture de l'inventaire de référence %s : %v\n", *windowsFile, err)
			os.Exit(1)
		}
		var referenceWindows []winapi.WindowInfo
		if err := json.Unmarshal(winData, &referenceWindows); err != nil {
			fmt.Fprintf(os.Stderr, "Inventaire de référence invalide : %v\n", err)
			os.Exit(1)
		}
		windowsRef, err := vdesktop.ValidateReference(entries, referenceWindows)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Inventaire de référence invalide : %v\n", err)
			os.Exit(1)
		}

		if !*execute {
			fmt.Println("=== MODE SIMULATION (aucun changement réel) ===")
			fmt.Println("Pour appliquer réellement, ajoutez l'option --execute.")
		}

		windows, err := winapi.EnumerateUserWindows()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture des fenêtres courantes : %v\n", err)
			os.Exit(1)
		}
		desktops, err := winapi.EnumerateVirtualDesktops()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture des bureaux courants : %v\n", err)
			os.Exit(1)
		}
		bilan, err := vdesktop.ApplyLayout(winapi.MoveWindowToDesktop, entries, windows, desktops, windowsRef, *execute)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lors de l'application : %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n--- Détails des opérations ---")
		for _, d := range bilan.Details {
			fmt.Println(" ", d)
		}

		fmt.Println("\n--- Bilan ---")
		fmt.Printf("Total fenêtres analysées : %d\n", bilan.TotalEntries)
		fmt.Printf("Réussis (ou simulés)     : %d\n", bilan.Success)
		fmt.Printf("Ignorés (fermés)         : %d\n", bilan.Ignored)
		fmt.Printf("Conflits (PID recyclés)  : %d\n", bilan.Conflicts)
		fmt.Printf("Échecs                   : %d\n", bilan.Failures)

	default:
		fmt.Fprintf(os.Stderr, "Sous-commande inconnue pour 'windows' : %q\n", subcmd)
		os.Exit(1)
	}
}

func handleDesktopCommand(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Erreur : sous-commande desktop requise (scan, plan, apply, undo)")
		os.Exit(1)
	}

	desktopDir, err := winapi.GetUserDesktopPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur détection du Bureau utilisateur : %v\n", err)
		os.Exit(1)
	}
	notifyOneDriveNotice(desktopDir)

	subcmd := args[0]
	switch subcmd {
	case "scan":
		fs := flag.NewFlagSet("desktop scan", flag.ExitOnError)
		outFile := fs.String("out", "desktop-files.json", "Fichier de sortie JSON")
		fs.Parse(args[1:])

		files, err := desktopfiles.ScanDesktop(desktopDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lors du scan du Bureau : %v\n", err)
			os.Exit(1)
		}

		data, err := json.MarshalIndent(files, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur sérialisation : %v\n", err)
			os.Exit(1)
		}

		if *outFile == "-" {
			fmt.Println(string(data))
		} else {
			if err := os.WriteFile(*outFile, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur écriture %s : %v\n", *outFile, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stdout, "Succès : %d fichier(s) éligible(s) recensé(s) sur le Bureau (%s) dans %s\n", len(files), desktopDir, *outFile)
		}

	case "plan":
		fs := flag.NewFlagSet("desktop plan", flag.ExitOnError)
		inputFile := fs.String("input", "desktop-files.json", "Fichier d'entrée du scan")
		rulesFile := fs.String("rules", "rules.json", "Fichier de règles JSON (optionnel)")
		outFile := fs.String("out", "desktop-plan.json", "Fichier de sortie du plan")
		fs.Parse(args[1:])

		inputData, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture fichier scan %s : %v\n", *inputFile, err)
			os.Exit(1)
		}

		var files []desktopfiles.ScannedFile
		if err := json.Unmarshal(inputData, &files); err != nil {
			fmt.Fprintf(os.Stderr, "Fichier scan invalide : %v\n", err)
			os.Exit(1)
		}

		rules := desktopfiles.DefaultRules()
		if *rulesFile != "" {
			if loaded, err := desktopfiles.LoadRules(*rulesFile); err == nil {
				rules = loaded
			}
		}

		plan, bilan, err := desktopfiles.GeneratePlan(files, rules, desktopDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur génération du plan : %v\n", err)
			os.Exit(1)
		}

		planData, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur sérialisation plan : %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(*outFile, planData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur écriture %s : %v\n", *outFile, err)
			os.Exit(1)
		}

		fmt.Println("--- Bilan du Plan de Rangement ---")
		fmt.Printf("Total fichiers traités : %d\n", bilan.Total)
		fmt.Printf("Déplacements prévus    : %d\n", bilan.Success)
		fmt.Printf("Conflits / Collisions  : %d\n", bilan.Conflicts)
		fmt.Printf("Ignorés                : %d\n", bilan.Ignored)
		fmt.Printf("Plan enregistré dans   : %s\n", *outFile)

	case "apply":
		fs := flag.NewFlagSet("desktop apply", flag.ExitOnError)
		planFile := fs.String("plan", "desktop-plan.json", "Fichier du plan JSON")
		execute := fs.Bool("execute", false, "Appliquer réellement les déplacements (simulation par défaut)")
		undoLog := fs.String("undo-log", "undo-log.json", "Fichier journal pour permettre un undo ultérieur")
		fs.Parse(args[1:])

		planData, err := os.ReadFile(*planFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture plan %s : %v\n", *planFile, err)
			os.Exit(1)
		}

		items, err := desktopfiles.ValidatePlanFile(planData, desktopDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Plan invalide ou non sécurisé : %v\n", err)
			os.Exit(1)
		}

		if !*execute {
			fmt.Println("=== MODE SIMULATION DU RANGEMENT (aucun fichier déplacé) ===")
			fmt.Println("Pour appliquer réellement, ajoutez l'option --execute.")
		}

		bilan, _, err := desktopfiles.ApplyPlan(items, desktopDir, *execute, *undoLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur exécution plan : %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n--- Détails des opérations ---")
		for _, d := range bilan.Details {
			fmt.Println(" ", d)
		}

		fmt.Println("\n--- Bilan ---")
		fmt.Printf("Total planifié         : %d\n", bilan.Total)
		fmt.Printf("Succès (ou simulés)    : %d\n", bilan.Success)
		fmt.Printf("Conflits (non écrasés) : %d\n", bilan.Conflicts)
		fmt.Printf("Ignorés (modifiés/abs) : %d\n", bilan.Ignored)
		fmt.Printf("Échecs                 : %d\n", bilan.Failures)
		if *execute && bilan.Success > 0 {
			fmt.Printf("Journal d'annulation sauvegardé dans : %s\n", *undoLog)
		}

	case "undo":
		fs := flag.NewFlagSet("desktop undo", flag.ExitOnError)
		logFile := fs.String("log", "undo-log.json", "Journal d'annulation JSON")
		execute := fs.Bool("execute", false, "Restaurer réellement les fichiers (simulation par défaut)")
		fs.Parse(args[1:])

		if !*execute {
			fmt.Println("=== MODE SIMULATION D'ANNULATION (aucun fichier restauré) ===")
			fmt.Println("Pour restaurer réellement, ajoutez l'option --execute.")
		}

		bilan, err := desktopfiles.ApplyUndo(*logFile, *execute)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur annulation : %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n--- Détails des opérations ---")
		for _, d := range bilan.Details {
			fmt.Println(" ", d)
		}

		fmt.Println("\n--- Bilan ---")
		fmt.Printf("Total à restaurer      : %d\n", bilan.Total)
		fmt.Printf("Succès (ou simulés)    : %d\n", bilan.Success)
		fmt.Printf("Conflits (non écrasés) : %d\n", bilan.Conflicts)
		fmt.Printf("Ignorés (disparus)     : %d\n", bilan.Ignored)
		fmt.Printf("Échecs                 : %d\n", bilan.Failures)

	default:
		fmt.Fprintf(os.Stderr, "Sous-commande inconnue pour 'desktop' : %q\n", subcmd)
		os.Exit(1)
	}
}
