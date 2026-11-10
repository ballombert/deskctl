package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"deskctl/internal/appconfig"
	"deskctl/internal/fancywm"
	"deskctl/internal/grouptracking"
	"deskctl/internal/tiling"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
	"deskctl/internal/windowstore"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type State struct {
	Plan      windoworganizer.Plan `json:"plan"`
	Rules     windowrules.Document `json:"rules"`
	Directory string               `json:"directory"`
	Config    appconfig.Config     `json:"config"`
	Displays  []Display            `json:"displays"`
	// FancyRunning tells whether FancyWM is running right now.
	FancyRunning  bool     `json:"fancyRunning"`
	Following     []string `json:"following"`
	TrackingError string   `json:"trackingError,omitempty"`
}

type Display struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	X            int32       `json:"x"`
	Y            int32       `json:"y"`
	Width        int32       `json:"width"`
	Height       int32       `json:"height"`
	Primary      bool        `json:"primary"`
	FancyWMIndex int         `json:"fancyWMIndex"`
	Work         winapi.Rect `json:"work"`
}

type Execution struct {
	Report windoworganizer.Report `json:"report"`
	Error  string                 `json:"error"`
}

type App struct {
	mu              sync.Mutex
	ctx             context.Context
	store           windowstore.Store
	config          appconfig.Config
	fancy           fancywm.Client
	engine          *windoworganizer.Engine
	state           State
	busy            atomic.Bool
	tracker         *grouptracking.Tracker
	trackingCancel  context.CancelFunc
	trackingDone    chan struct{}
	geometryStarted bool
}

func New() (*App, error) {
	store, err := windowstore.Portable()
	if err != nil {
		return nil, err
	}
	config, err := appconfig.Load(filepath.Join(store.Directory, "deskctl.yaml"))
	if err != nil {
		return nil, err
	}
	return &App{store: store, engine: windoworganizer.New(windoworganizer.WindowsBackend{}, store),
		config: config, fancy: fancywm.NewClient(),
		state: State{Rules: windowrules.Empty(), Directory: store.Directory, Config: config, Displays: []Display{}}}, nil
}

func Startup(app *App) func(context.Context) {
	return func(ctx context.Context) { app.ctx = ctx }
}

func Shutdown(app *App) func(context.Context) {
	return func(context.Context) {
		app.mu.Lock()
		app.stopFollowing()
		if app.trackingCancel != nil {
			app.trackingCancel()
		}
		done := app.trackingDone
		app.mu.Unlock()
		if done != nil {
			<-done
		}
	}
}

func BeforeClose(app *App) func(context.Context) bool {
	return func(ctx context.Context) bool {
		if !app.busy.Load() {
			return false
		}
		_, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type: runtime.WarningDialog, Title: "Operation en cours",
			Message: "Attendez le bilan avant de fermer. Les mouvements deja effectues ne sont pas annules par la fermeture.",
		})
		if err != nil {
			runtime.LogError(ctx, err.Error())
		}
		return true
	}
}

func (app *App) Refresh() (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.tracker != nil {
		app.tracker.Cancel()
	}
	displays, err := app.listDisplays()
	if err != nil {
		return State{}, err
	}
	doc, err := app.store.LoadRules()
	if err != nil {
		return State{}, err
	}
	app.engine.SetGap(app.config.Tiling.Gap)
	plan, err := app.engine.Prepare(doc)
	if err != nil {
		return State{}, err
	}
	app.state = State{Plan: plan, Rules: doc, Directory: app.store.Directory, Config: app.config, Displays: displays,
		FancyRunning: app.fancyCLI().Running(), Following: app.state.Following, TrackingError: app.state.TrackingError}
	return app.state, nil
}

func (app *App) listDisplays() ([]Display, error) {
	physical, err := winapi.EnumerateDisplays()
	if err != nil {
		return nil, err
	}
	displays := make([]Display, 0, len(physical))
	for _, screen := range physical {
		name := strings.TrimPrefix(screen.Name, `\\.\`)
		displays = append(displays, Display{
			ID: screen.ID, Name: name, X: screen.X, Y: screen.Y,
			Width: screen.Width, Height: screen.Height, Primary: screen.Primary,
			FancyWMIndex: app.config.FancyWM.DisplayIndices[screen.ID], Work: screen.Work,
		})
	}
	return displays, nil
}

func (app *App) Assign(planID, windowID, desktopID string) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	plan, err := app.engine.Assign(planID, windowID, desktopID)
	if err != nil {
		return State{}, err
	}
	app.state.Plan = plan
	return app.state, nil
}

func (app *App) MoveToDisplay(planID, windowID, displayID string) error {
	app.mu.Lock()
	defer app.mu.Unlock()
	if planID == "" || app.state.Plan.ID != planID || app.state.Plan.Mode != "arrange" {
		return fmt.Errorf("apercu obsolete : actualiser")
	}
	var expected *windoworganizer.Window
	for _, item := range app.state.Plan.Items {
		if item.Window.ID == windowID {
			window := item.Window
			expected = &window
			break
		}
	}
	if expected == nil || expected.Blocked != "" {
		return fmt.Errorf("fenetre absente ou non deplacable")
	}
	var target, source *Display
	for index := range app.state.Displays {
		switch app.state.Displays[index].ID {
		case displayID:
			target = &app.state.Displays[index]
		case expected.DisplayID:
			source = &app.state.Displays[index]
		}
	}
	if target == nil {
		return fmt.Errorf("ecran absent : actualiser")
	}
	index := target.FancyWMIndex
	native := index == 0 || expected.Pinned || !app.fancyCLI().Running()
	hwnd, err := strconv.ParseUint(expected.ID, 10, strconv.IntSize)
	if err != nil || hwnd == 0 {
		return fmt.Errorf("identite de fenetre invalide")
	}
	identity, err := winapi.ReadWindowIdentity(uintptr(hwnd))
	if err != nil {
		return err
	}
	pinned, err := winapi.IsWindowPinned(uintptr(hwnd))
	if err != nil {
		return err
	}
	if identity.ProcessID != expected.ProcessID || !strings.EqualFold(identity.Application, expected.Application) ||
		identity.Class != expected.Class || identity.Started != expected.Started ||
		identity.DesktopID != expected.DesktopID || pinned != expected.Pinned {
		return fmt.Errorf("fenetre modifiee depuis l’apercu : actualiser")
	}
	app.busy.Store(true)
	defer app.busy.Store(false)
	if native {
		// Without FancyWM the window keeps its relative position and size on the new screen's work area.
		bounds, err := winapi.VisibleWindowRect(uintptr(hwnd))
		if err != nil {
			return err
		}
		from := winapi.Rect{}
		if source != nil {
			from = source.Work
		}
		return winapi.SetVisibleWindowRect(uintptr(hwnd), windoworganizer.Rescale(bounds, from, target.Work))
	}
	return winapi.WithForegroundWindow(uintptr(hwnd), func() error {
		return app.fancy.MoveToDisplay(app.ctx, app.config.FancyWM.Executable, index)
	})
}

func (app *App) OpenDisplaySettings() {
	runtime.BrowserOpenURL(app.ctx, "ms-settings:display")
}

func (app *App) SaveRules(doc windowrules.Document, planID string) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.state.Plan.ID != planID {
		return State{}, fmt.Errorf("apercu obsolete : actualiser avant d'enregistrer")
	}
	doc, err := windowrules.Normalize(doc)
	if err != nil {
		return State{}, err
	}
	if err := app.store.SaveRules(doc); err != nil {
		return State{}, err
	}
	plan, err := app.engine.SetRules(doc)
	if err != nil {
		return State{}, fmt.Errorf("regles enregistrees mais apercu indisponible : actualiser : %w", err)
	}
	app.state.Plan, app.state.Rules = plan, doc
	app.stopFollowing()
	return app.state, nil
}

func (app *App) PreviewRule(rule windowrules.Rule) (RulePreview, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	return previewRule(app.state.Plan, rule, app.config.Tiling.Gap, app.state.Rules.Groups...)
}

func (app *App) PreviewUndo() (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	plan, err := app.engine.PreviewUndo()
	if err != nil {
		return State{}, err
	}
	app.state.Plan = plan
	return app.state, nil
}

func (app *App) Execute(planID string) Execution { return app.run(planID, nil) }

// ApplyRule applies one rule: desktop moves first, then its window layout.
func (app *App) ApplyRule(planID, ruleID string) Execution { return app.run(planID, []string{ruleID}) }

// ApplyAll applies every enabled rule and manual choice of the plan.
func (app *App) ApplyAll(planID string) Execution { return app.run(planID, nil) }

func (app *App) fancyCLI() tiling.FancyCLI {
	return tiling.FancyCLI{Context: app.ctx, Client: app.fancy, Executable: app.config.FancyWM.Executable}
}

func (app *App) run(planID string, ruleIDs []string) Execution {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.busy.Store(true)
	defer app.busy.Store(false)
	if app.tracker != nil {
		app.tracker.Cancel()
	}
	plan := app.state.Plan
	for _, layout := range plan.Layouts {
		if layout.Status == "pending" && (len(ruleIDs) == 0 || slices.Contains(ruleIDs, layout.RuleID) || hasRuleAlias(ruleIDs, layout.RuleIDs)) {
			if err := app.geometryReady(); err != nil {
				return Execution{Error: err.Error(), Report: windoworganizer.Report{Results: []windoworganizer.Result{}}}
			}
			break
		}
	}
	report, err := app.engine.Apply(planID, ruleIDs...)
	app.state.Plan = windoworganizer.Plan{}
	if errors.Is(err, windoworganizer.ErrNothing) && plan.Mode == "arrange" && len(report.Layouts) != 0 {
		err = nil
	}
	result := Execution{Report: report}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for index := range result.Report.Results {
		result.Report.Results[index].Kind = "desktop"
	}
	jobs := layoutJobs(plan, report, app.config.FancyWM.DisplayIndices)
	if len(jobs) == 0 {
		return result
	}
	if saveErr := app.store.SaveGeometry(geometryUndo(jobs, plan)); saveErr != nil {
		result.Error = "disposition non appliquee : sauvegarde des positions impossible : " + saveErr.Error()
		return result
	}
	fancy := app.fancyCLI()
	uncertain := false
	for _, job := range jobs {
		desktop, checkErr := checkedJob(job, plan)
		if checkErr != nil {
			uncertain = true
			result.Report.Results = append(result.Report.Results, windoworganizer.Result{ID: job.Layout.GroupID,
				Title: job.Layout.RuleName, Kind: "layout", Status: "failed", Message: checkErr.Error()})
			continue
		}
		for _, outcome := range tiling.Apply(app.config.Tiling.Engine, desktop, fancy, job) {
			if outcome.Status != "success" {
				uncertain = true
			}
			result.Report.Results = append(result.Report.Results, windoworganizer.Result{ID: outcome.WindowID, Title: outcome.Title,
				Status: outcome.Status, Message: job.Layout.RuleName + " : " + outcome.Message, Kind: "layout", Engine: outcome.Engine})
			if outcome.Status == "success" {
				if captureErr := app.captureGeometryAfter(outcome.WindowID); captureErr != nil {
					result.Error = "verification du journal de positions : " + captureErr.Error()
					return result
				}
			}
		}
	}
	if err := app.finishGeometry(uncertain); err != nil {
		result.Error = "journal de positions non finalise : " + err.Error()
	}
	return result
}

// RestoreGeometry puts back the windows moved by the last layout, when they are still the same windows.
func (app *App) RestoreGeometry() Execution {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.busy.Store(true)
	defer app.busy.Store(false)
	if app.tracker != nil {
		app.tracker.Cancel()
	}
	result := Execution{Report: windoworganizer.Report{Results: []windoworganizer.Result{}}}
	undo, err := app.store.LoadGeometry()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	restored := true
	for _, record := range undo.Records {
		outcome := restoreRecord(record, undo.Version)
		if outcome.Status != "success" {
			restored = false
		}
		result.Report.Results = append(result.Report.Results, outcome)
	}
	if restored {
		undo.InProgress, undo.Uncertain = false, false
		if err := app.store.SaveGeometry(undo); err != nil {
			result.Error = err.Error()
		}
	}
	app.engine.Invalidate()
	app.state.Plan.ID = ""
	return result
}

// CaptureLayout drafts a rule from the current position of the selected windows (not saved).
func (app *App) CaptureLayout(windowIDs []string, axis string) (windowrules.Rule, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return windowrules.Rule{}, err
	}
	return captureRule(app.state.Plan, windowIDs, axis, "r-"+hex.EncodeToString(data))
}

// SetDisplayIndex links a screen to its FancyWM index (0 removes it) and saves deskctl.yaml.
func (app *App) SetDisplayIndex(displayID string, index int) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	config := app.config
	config.FancyWM.DisplayIndices = map[string]int{}
	for key, value := range app.config.FancyWM.DisplayIndices {
		if key != displayID && (index == 0 || value != index) {
			config.FancyWM.DisplayIndices[key] = value
		}
	}
	if index != 0 {
		config.FancyWM.DisplayIndices[displayID] = index
	}
	return app.saveConfig(config)
}

// SetTiling chooses the tiling engine (auto, fancywm, native) and the gap in pixels.
func (app *App) SetTiling(engine string, gap int) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	config := app.config
	config.Tiling = appconfig.Tiling{Engine: engine, Gap: int32(gap)}
	return app.saveConfig(config)
}

func (app *App) saveConfig(config appconfig.Config) (State, error) {
	if err := appconfig.Save(filepath.Join(app.store.Directory, "deskctl.yaml"), config); err != nil {
		return State{}, err
	}
	app.config = config
	app.engine.SetGap(config.Tiling.Gap)
	displays, err := app.listDisplays()
	if err != nil {
		return State{}, err
	}
	app.state.Config, app.state.Displays = config, displays
	return app.state, nil
}

func (app *App) ImportRules() (windowrules.Document, error) {
	path, err := runtime.OpenFileDialog(app.ctx, runtime.OpenDialogOptions{
		Title:   "Importer des regles (apercu avant enregistrement)",
		Filters: []runtime.FileFilter{{DisplayName: "Regles JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return windowrules.Document{}, err
	}
	if path == "" {
		return windowrules.Document{}, fmt.Errorf("import annule")
	}
	var doc windowrules.Document
	if err := windowstore.Read(path, &doc); err != nil {
		return doc, err
	}
	return windowrules.Normalize(doc)
}

func (app *App) ExportRules() (string, error) {
	app.mu.Lock()
	doc := app.state.Rules
	app.mu.Unlock()
	doc, err := windowrules.Normalize(doc)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(app.ctx, runtime.SaveDialogOptions{
		Title: "Exporter les regles vers un nouveau fichier", DefaultFilename: "window-rules-export.json",
		Filters: []runtime.FileFilter{{DisplayName: "Regles JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("export annule")
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("export refuse, aucun ecrasement : %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		removeErr := os.Remove(path)
		return "", fmt.Errorf("export incomplet : ecriture=%v fermeture=%v nettoyage=%v", writeErr, closeErr, removeErr)
	}
	return path, nil
}
