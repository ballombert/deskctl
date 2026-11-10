package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"deskctl/internal/grouptracking"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
	"deskctl/internal/windowstore"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (app *App) AssignGroup(planID, groupID, desktopID, displayID string, zone windowrules.Zone) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	plan, err := app.engine.AssignGroup(planID, groupID, desktopID, displayID, zone)
	if err != nil {
		return State{}, err
	}
	app.state.Plan = plan
	return app.state, nil
}

func (app *App) PreviewGroup(group windowrules.Group) (RulePreview, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	groups := []windowrules.Group{group}
	for _, current := range app.state.Rules.Groups {
		if current.ID != group.ID {
			groups = append(groups, current)
		}
	}
	desktop, display := app.state.Plan.CurrentDesktopID, ""
	for _, current := range app.state.Plan.Groups {
		if current.ID == group.ID {
			desktop, display = current.DesktopID, current.DisplayID
		}
	}
	if desktop == "" && len(app.state.Plan.Desktops) != 0 {
		desktop = app.state.Plan.Desktops[0].ID
	}
	rule := windowrules.Rule{ID: "preview", Name: group.Name, Enabled: true,
		GroupID: group.ID, DesktopID: desktop, DisplayID: display}
	if strings.TrimSpace(groups[0].Name) == "" {
		groups[0].Name = "Apercu"
	}
	return previewRule(app.state.Plan, rule, app.config.Tiling.Gap, groups...)
}

type groupEvent struct {
	GroupID string `json:"groupId"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (app *App) emitGroup(event groupEvent) {
	if app.ctx != nil {
		runtime.EventsEmit(app.ctx, "groups:changed", event)
	}
}

func (app *App) stopFollowing() {
	if app.tracker != nil {
		app.tracker.Cancel()
	}
	app.state.Following = []string{}
	app.state.TrackingError = ""
}

func (app *App) trackingGroups() ([]grouptracking.Group, error) {
	inventory, err := (windoworganizer.WindowsBackend{}).Inventory()
	if err != nil {
		return nil, err
	}
	result := []grouptracking.Group{}
	for _, group := range app.state.Rules.Groups {
		if !slices.Contains(app.state.Following, group.ID) {
			continue
		}
		matcher := windowrules.Rule{Enabled: true, Layout: &group.Layout}
		current := grouptracking.Group{ID: group.ID, Members: []grouptracking.Member{}}
		for _, window := range inventory.Windows {
			if matcher.Matches(window.Application, window.Title) {
				if window.Blocked != "" || window.Elevated {
					return nil, fmt.Errorf("groupe %s : fenetre indisponible ou administrateur : %s", group.Name, window.Title)
				}
				matches := 0
				for _, other := range app.state.Rules.Groups {
					if (windowrules.Rule{Enabled: true, Layout: &other.Layout}).Matches(window.Application, window.Title) {
						matches++
					}
				}
				if matches > 1 {
					return nil, fmt.Errorf("fenetre %s : appartenance a plusieurs groupes", window.Title)
				}
				current.Members = append(current.Members, memberOf(window))
			}
		}
		if len(current.Members) != 0 {
			result = append(result, current)
		}
	}
	return result, nil
}

func memberOf(window windoworganizer.Window) grouptracking.Member {
	return grouptracking.Member{ID: window.ID, ProcessID: window.ProcessID, Started: window.Started,
		Application: window.Application, Class: window.Class, DesktopID: window.DesktopID,
		Pinned: window.Pinned, Rect: window.Bounds}
}

func (app *App) SetGroupFollowing(groupID string, enabled bool) (State, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if !enabled {
		app.state.Following = slices.DeleteFunc(app.state.Following, func(id string) bool { return id == groupID })
		if app.tracker != nil {
			app.tracker.Cancel()
		}
		return app.state, nil
	}
	if app.ctx == nil {
		return State{}, fmt.Errorf("suivi indisponible : application non demarree")
	}
	found := false
	for _, group := range app.state.Plan.Groups {
		if group.ID == groupID {
			if group.Status == "conflict" || len(group.Members) == 0 {
				return State{}, fmt.Errorf("groupe non eligible au suivi : %s", group.Explanation)
			}
			found = true
		}
	}
	if !found {
		return State{}, fmt.Errorf("groupe absent de l'apercu : actualiser")
	}
	if err := app.geometryReady(); err != nil {
		return State{}, err
	}
	if app.fancyCLI().Running() {
		return State{}, fmt.Errorf("suivi indisponible pendant que FancyWM est actif : arreter FancyWM pour suivre les gestes Windows")
	}
	previous := append([]string{}, app.state.Following...)
	if !slices.Contains(app.state.Following, groupID) {
		app.state.Following = append(app.state.Following, groupID)
	}
	groups, err := app.trackingGroups()
	if err != nil {
		app.state.Following = previous
		return State{}, err
	}
	if app.tracker == nil {
		app.tracker = grouptracking.New(trackingBackend{app})
	}
	if err := app.tracker.SetGroups(groups); err != nil {
		app.state.Following = previous
		return State{}, err
	}
	if app.trackingCancel == nil {
		if err := app.startTracking(); err != nil {
			app.state.Following = previous
			return State{}, err
		}
	}
	app.state.TrackingError = ""
	return app.state, nil
}

func (app *App) startTracking() error {
	ctx, cancel := context.WithCancel(app.ctx)
	events := make(chan winapi.WindowMoveEvent, 128)
	overflow := make(chan struct{}, 1)
	done, err := winapi.StartWindowMoveWatcher(ctx, func(event winapi.WindowMoveEvent) {
		if app.busy.Load() {
			return
		}
		select {
		case events <- event:
		default:
			select {
			case overflow <- struct{}{}:
			default:
			}
			cancel()
		}
	})
	if err != nil {
		cancel()
		return err
	}
	app.trackingCancel = cancel
	app.trackingDone = make(chan struct{})
	stopped := app.trackingDone
	go func() {
		defer close(stopped)
		for {
			select {
			case event := <-events:
				app.handleMove(event)
			case <-overflow:
				app.trackingFailure(fmt.Errorf("suivi interrompu : trop d'evenements Windows"))
				<-done
				return
			case err := <-done:
				app.mu.Lock()
				app.trackingCancel = nil
				app.mu.Unlock()
				select {
				case <-overflow:
					err = fmt.Errorf("suivi interrompu : trop d'evenements Windows")
				default:
				}
				if err != nil {
					app.trackingFailure(err)
				}
				return
			}
		}
	}()
	return nil
}

func (app *App) trackingFailure(err error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.stopFollowing()
	app.trackingCancel = nil
	app.state.TrackingError = err.Error()
	app.engine.Invalidate()
	app.state.Plan.ID = ""
	app.emitGroup(groupEvent{Error: err.Error()})
}

func (app *App) handleMove(event winapi.WindowMoveEvent) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.state.Following) == 0 || app.busy.Load() {
		return
	}
	if event.Start {
		groups, err := app.trackingGroups()
		tracked := false
		for _, group := range groups {
			for _, member := range group.Members {
				tracked = tracked || member.ID == event.WindowID
			}
		}
		if tracked || err != nil {
			app.engine.Invalidate()
			app.state.Plan.ID = ""
		}
		if err == nil {
			err = app.tracker.SetGroups(groups)
		}
		if err == nil {
			err = app.tracker.Begin(event.WindowID)
		}
		if err != nil {
			app.stopFollowing()
			app.state.TrackingError = err.Error()
			app.emitGroup(groupEvent{Error: err.Error()})
		}
		return
	}
	app.busy.Store(true)
	app.geometryStarted = false
	id, changed, err := app.tracker.End(event.WindowID)
	app.busy.Store(false)
	if id == "" && !changed && err == nil {
		return
	}
	// Any recognized source gesture invalidates execution, including a rejected drag.
	app.engine.Invalidate()
	app.state.Plan.ID = ""
	if app.geometryStarted {
		if finishErr := app.finishGeometry(err != nil); finishErr != nil {
			err = errors.Join(err, finishErr)
		}
	}
	if err != nil {
		app.state.Following = slices.DeleteFunc(app.state.Following, func(group string) bool { return group == id })
		app.state.TrackingError = err.Error()
		app.emitGroup(groupEvent{GroupID: id, Error: err.Error()})
	} else {
		app.emitGroup(groupEvent{GroupID: id, Message: "Geste du groupe termine : actualiser l'apercu avant application."})
	}
}

type trackingBackend struct{ app *App }

func (trackingBackend) Read(id string) (grouptracking.Member, error) {
	hwnd, err := strconv.ParseUint(id, 10, strconv.IntSize)
	if err != nil || hwnd == 0 {
		return grouptracking.Member{}, fmt.Errorf("identite de fenetre invalide")
	}
	identity, err := winapi.ReadWindowIdentity(uintptr(hwnd))
	if err != nil {
		return grouptracking.Member{}, err
	}
	pinned, err := winapi.IsWindowPinned(uintptr(hwnd))
	if err != nil {
		return grouptracking.Member{}, err
	}
	rect, err := winapi.VisibleWindowRect(uintptr(hwnd))
	if err != nil {
		return grouptracking.Member{}, err
	}
	return grouptracking.Member{ID: id, ProcessID: identity.ProcessID, Started: identity.Started,
		Application: identity.Application, Class: identity.Class, DesktopID: identity.DesktopID, Pinned: pinned, Rect: rect}, nil
}

func (trackingBackend) Areas() ([]winapi.Rect, error) {
	displays, err := winapi.EnumerateDisplays()
	if err != nil {
		return nil, err
	}
	areas := []winapi.Rect{}
	for _, display := range displays {
		areas = append(areas, display.Work)
	}
	return areas, nil
}

func (backend trackingBackend) Place(expected grouptracking.Member, rect winapi.Rect) error {
	if backend.app.fancyCLI().Running() {
		return fmt.Errorf("suivi suspendu : FancyWM actif ; utiliser les commandes deskctl ou arreter FancyWM")
	}
	actual, err := backend.Read(expected.ID)
	if err != nil {
		return err
	}
	if !sameMember(expected, actual) || actual.Rect != expected.Rect {
		return fmt.Errorf("fenetre modifiee avant la translation")
	}
	hwnd, err := strconv.ParseUint(expected.ID, 10, strconv.IntSize)
	if err != nil {
		return err
	}
	return winapi.SetVisibleWindowRect(uintptr(hwnd), rect)
}

func sameMember(expected, actual grouptracking.Member) bool {
	return expected.ID == actual.ID && expected.ProcessID == actual.ProcessID && expected.Started == actual.Started &&
		strings.EqualFold(expected.Application, actual.Application) && expected.Class == actual.Class &&
		(expected.Pinned || expected.DesktopID == actual.DesktopID) && expected.Pinned == actual.Pinned
}

func (backend trackingBackend) Save(before, after []grouptracking.Member) error {
	if len(before) == 0 || len(before) != len(after) {
		return fmt.Errorf("positions de groupe incompletes")
	}
	if err := backend.app.geometryReady(); err != nil {
		return err
	}
	undo := windowstore.GeometryUndo{Version: 2, InProgress: true, Records: []windowstore.GeometryRecord{}}
	for index, member := range before {
		if !sameMember(member, after[index]) {
			return fmt.Errorf("identites du groupe incoherentes")
		}
		rect := after[index].Rect
		undo.Records = append(undo.Records, windowstore.GeometryRecord{WindowID: member.ID, ProcessID: member.ProcessID,
			Started: member.Started, Application: member.Application, Class: member.Class,
			DesktopID: member.DesktopID, Pinned: member.Pinned, Rect: member.Rect, After: &rect})
	}
	if err := backend.app.store.SaveGeometry(undo); err != nil {
		return err
	}
	backend.app.geometryStarted = true
	return nil
}

func (app *App) geometryReady() error {
	undo, err := app.store.LoadGeometry()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if undo.InProgress || undo.Uncertain {
		return fmt.Errorf("positions precedentes incertaines : verifier et restaurer ou archiver window-geometry-undo.json avant un nouveau geste")
	}
	return nil
}

func (app *App) finishGeometry(uncertain bool) error {
	undo, err := app.store.LoadGeometry()
	if err != nil {
		return err
	}
	if !undo.InProgress {
		return nil
	}
	undo.InProgress, undo.Uncertain = false, uncertain
	return app.store.SaveGeometry(undo)
}

func (app *App) captureGeometryAfter(id string) error {
	undo, err := app.store.LoadGeometry()
	if err != nil {
		return err
	}
	actual, err := (trackingBackend{}).Read(id)
	if err != nil {
		return err
	}
	found := false
	for index := range undo.Records {
		record := &undo.Records[index]
		if record.WindowID != id {
			continue
		}
		expected := grouptracking.Member{ID: id, ProcessID: record.ProcessID, Started: record.Started,
			Application: record.Application, Class: record.Class, DesktopID: record.DesktopID, Pinned: record.Pinned}
		if !sameMember(expected, actual) {
			return fmt.Errorf("identite changee apres le placement")
		}
		rect := actual.Rect
		record.After = &rect
		found = true
		break
	}
	if !found {
		return fmt.Errorf("fenetre sans entree de journal")
	}
	return app.store.SaveGeometry(undo)
}
