package windoworganizer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

type Desktop struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Window struct {
	ID          string `json:"id"`
	ProcessID   uint32 `json:"processId"`
	Application string `json:"application"`
	Title       string `json:"title"`
	Class       string `json:"class"`
	Started     string `json:"started"`
	DesktopID   string `json:"desktopId"`
	Pinned      bool   `json:"pinned"`
	Blocked     string `json:"blocked"`
	// BlockCode classifies Blocked: identity, pin, no-desktop, immovable.
	BlockCode string      `json:"blockCode,omitempty"`
	DisplayID string      `json:"displayId,omitempty"`
	Bounds    winapi.Rect `json:"bounds"`
	// Elevated windows can change desktop but cannot be resized by a non-admin deskctl.
	Elevated bool `json:"elevated,omitempty"`
}

type Inventory struct {
	Desktops         []Desktop            `json:"desktops"`
	Displays         []winapi.DisplayInfo `json:"displays"`
	CurrentDesktopID string               `json:"currentDesktopId"`
	Windows          []Window             `json:"windows"`
}

type Item struct {
	Window      Window   `json:"window"`
	TargetID    string   `json:"targetId"`
	Source      string   `json:"source"`
	Status      string   `json:"status"`
	Explanation string   `json:"explanation"`
	RuleIDs     []string `json:"ruleIds"`
}

type Plan struct {
	ID               string               `json:"id"`
	Mode             string               `json:"mode"`
	Desktops         []Desktop            `json:"desktops"`
	Displays         []winapi.DisplayInfo `json:"displays"`
	CurrentDesktopID string               `json:"currentDesktopId"`
	Items            []Item               `json:"items"`
	Layouts          []LayoutPlan         `json:"layouts"`
	Groups           []GroupState         `json:"groups"`
}

type GroupState struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Members     []string    `json:"members"`
	DisplayID   string      `json:"displayId"`
	DesktopID   string      `json:"desktopId"`
	Bounds      winapi.Rect `json:"bounds"`
	Status      string      `json:"status"`
	Explanation string      `json:"explanation"`
}

// ErrNothing reports that a batch contains no desktop move.
var ErrNothing = errors.New("aucun mouvement a appliquer")

type Record struct {
	Before Window `json:"before"`
	Target string `json:"target"`
	State  string `json:"state"`
}

type Journal struct {
	Version int      `json:"version"`
	Batch   string   `json:"batch"`
	Records []Record `json:"records"`
}

type Result struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// Kind is "desktop" (default) or "layout"; Engine is set for layouts.
	Kind   string `json:"kind,omitempty"`
	Engine string `json:"engine,omitempty"`
}

type Report struct {
	Results  []Result `json:"results"`
	Recovery *Journal `json:"recovery,omitempty"`
	// Layouts lists the geometry still to apply once desktop moves are done.
	Layouts []LayoutPlan `json:"layouts,omitempty"`
}

type Backend interface {
	Inventory() (Inventory, error)
	Move(Window, string) error
}

type Journals interface {
	LoadJournal() (Journal, error)
	SaveJournal(Journal) error
}

type Engine struct {
	mu           sync.Mutex
	backend      Backend
	journals     Journals
	rules        windowrules.Document
	current      Plan
	manual       map[string]string
	manualGroups map[string]windowrules.Rule
	undo         Journal
	gap          int32
}

func New(backend Backend, journals Journals) *Engine {
	return &Engine{backend: backend, journals: journals, gap: 8}
}

// SetGap sets the space in pixels between tiled windows.
func (engine *Engine) SetGap(gap int32) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.gap = max(gap, 0)
}

func token() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func (engine *Engine) Prepare(doc windowrules.Document) (Plan, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.current = Plan{}
	doc, err := windowrules.Normalize(doc)
	if err != nil {
		return Plan{}, err
	}
	inventory, err := engine.backend.Inventory()
	if err != nil {
		return Plan{}, err
	}
	id, err := token()
	if err != nil {
		return Plan{}, err
	}
	engine.rules, engine.manual = doc, map[string]string{}
	engine.manualGroups = map[string]windowrules.Rule{}
	engine.current = Plan{ID: id, Mode: "arrange", Desktops: inventory.Desktops, Displays: inventory.Displays,
		CurrentDesktopID: inventory.CurrentDesktopID, Items: []Item{}}
	for _, window := range inventory.Windows {
		engine.current.Items = append(engine.current.Items, Item{Window: window})
	}
	for index, item := range engine.current.Items {
		engine.current.Items[index] = engine.evaluate(item.Window)
	}
	engine.rebuildLayouts()
	return clonePlan(engine.current), nil
}

func hasDesktop(desktops []Desktop, id string) bool {
	for _, desktop := range desktops {
		if desktop.ID == id {
			return true
		}
	}
	return false
}

func hasDestination(desktops []Desktop, id string) bool {
	return id == winapi.AllDesktopsID || hasDesktop(desktops, id)
}

func (engine *Engine) evaluate(window Window) Item {
	item := Item{Window: window, Status: "unmatched", Explanation: "Aucune regle : reste sur son bureau", RuleIDs: []string{}}
	if window.Blocked != "" {
		item.Status, item.Explanation = "blocked", window.Blocked
		return item
	}
	geometry := []string{}
	geometrySeen := map[string]bool{}
	for _, rule := range engine.effectiveRules() {
		if rule.Matches(window.Application, window.Title) {
			item.RuleIDs = append(item.RuleIDs, rule.ID)
			if rule.HasGeometry() && !geometrySeen[geometryKey(rule)] {
				geometry = append(geometry, rule.Name)
				geometrySeen[geometryKey(rule)] = true
			}
		}
	}
	if len(geometry) > 1 {
		item.Status, item.Explanation = "conflict", "Plusieurs dispositions pour cette fenetre : "+strings.Join(geometry, ", ")
		return item
	}
	matchingGroups := []string{}
	for _, group := range engine.rules.Groups {
		if (windowrules.Rule{Enabled: true, Layout: &group.Layout}).Matches(window.Application, window.Title) {
			matchingGroups = append(matchingGroups, group.Name)
		}
	}
	if len(matchingGroups) > 1 {
		item.Status, item.Explanation = "conflict", "Plusieurs groupes pour cette fenetre : "+strings.Join(matchingGroups, ", ")
		return item
	}
	for _, rule := range engine.effectiveRules() {
		if !rule.Enabled || rule.GroupID == "" || !rule.Matches(window.Application, window.Title) {
			continue
		}
		for _, other := range engine.current.Items {
			if other.Window.Blocked != "" && rule.Matches(other.Window.Application, other.Window.Title) {
				item.Status, item.Explanation = "conflict", "Groupe incomplet : "+other.Window.Blocked
				return item
			}
		}
	}
	if target, found := engine.manual[window.ID]; found {
		item.TargetID, item.Source, item.Explanation = target, "manual", "Choix manuel pour ce lot uniquement"
	} else {
		names := []string{}
		for _, rule := range engine.effectiveRules() {
			if !rule.Matches(window.Application, window.Title) {
				continue
			}
			names = append(names, rule.Name)
			if !hasDestination(engine.current.Desktops, rule.DesktopID) {
				item.TargetID, item.Status, item.Explanation = "", "conflict", "Bureau absent pour la regle : "+rule.Name
				return item
			}
			if item.TargetID != "" && item.TargetID != rule.DesktopID {
				item.TargetID, item.Status, item.Explanation = "", "conflict", "Destinations contradictoires : "+strings.Join(names, ", ")
				return item
			}
			item.TargetID, item.Source = rule.DesktopID, "rule"
			if _, manual := engine.manualGroups[rule.GroupID]; manual {
				item.Source = "manual"
			}
		}
		if len(names) != 0 {
			item.Explanation = "Regles : " + strings.Join(names, ", ")
		}
	}
	if item.TargetID != "" {
		item.Status = "pending"
		if item.TargetID == window.DesktopID {
			item.Status = "unchanged"
		}
	}
	return item
}

func clonePlan(plan Plan) Plan {
	plan.Items = append([]Item{}, plan.Items...)
	plan.Desktops = append([]Desktop{}, plan.Desktops...)
	plan.Displays = append([]winapi.DisplayInfo{}, plan.Displays...)
	plan.Layouts = append([]LayoutPlan{}, plan.Layouts...)
	plan.Groups = append([]GroupState{}, plan.Groups...)
	return plan
}

func (engine *Engine) Assign(planID, windowID, target string) (Plan, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if planID == "" || engine.current.ID != planID || engine.current.Mode != "arrange" {
		return Plan{}, fmt.Errorf("apercu obsolete : actualiser")
	}
	if target != "" && !hasDestination(engine.current.Desktops, target) {
		return Plan{}, fmt.Errorf("bureau cible absent")
	}
	for index, item := range engine.current.Items {
		if item.Window.ID != windowID {
			continue
		}
		if item.Window.Blocked != "" {
			return Plan{}, fmt.Errorf("fenetre non deplacable : %s", item.Window.Blocked)
		}
		revision, err := token()
		if err != nil {
			return Plan{}, err
		}
		if target == "" {
			delete(engine.manual, windowID)
		} else {
			engine.manual[windowID] = target
		}
		engine.current.Items[index] = engine.evaluate(item.Window)
		engine.rebuildLayouts()
		engine.current.ID = revision
		return clonePlan(engine.current), nil
	}
	return Plan{}, fmt.Errorf("fenetre absente de l'apercu")
}

func (engine *Engine) SetRules(doc windowrules.Document) (Plan, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	normalized, err := windowrules.Normalize(doc)
	if err != nil {
		return Plan{}, err
	}
	revision, err := token()
	if err != nil {
		return Plan{}, err
	}
	engine.rules = normalized
	for id, manual := range engine.manualGroups {
		retained := false
		for _, group := range normalized.Groups {
			if group.ID == id {
				retained = true
				break
			}
		}
		if !retained {
			delete(engine.manualGroups, id)
			continue
		}
		doc, err := windowrules.Normalize(windowrules.Document{Version: windowrules.Version,
			Groups: normalized.Groups, Rules: []windowrules.Rule{manual}})
		if err != nil {
			return Plan{}, err
		}
		engine.manualGroups[id] = doc.Rules[0]
	}
	if engine.current.Mode == "arrange" {
		for index, item := range engine.current.Items {
			engine.current.Items[index] = engine.evaluate(item.Window)
		}
		engine.rebuildLayouts()
		engine.current.ID = revision
	}
	return clonePlan(engine.current), nil
}

// sameWindow checks the stable identity of a window. The title is excluded on
// purpose: browsers, chats and editors change it constantly, which used to make
// those windows look "modified" and therefore impossible to move.
func sameWindow(expected, actual Window) bool {
	return expected.ID == actual.ID && expected.ProcessID == actual.ProcessID &&
		expected.Started != "" && expected.Started == actual.Started &&
		strings.EqualFold(expected.Application, actual.Application) &&
		expected.Class == actual.Class &&
		expected.DesktopID == actual.DesktopID && expected.Pinned == actual.Pinned && actual.Blocked == ""
}

func findWindow(inventory Inventory, id string) (Window, bool) {
	for _, window := range inventory.Windows {
		if window.ID == id {
			return window, true
		}
	}
	return Window{}, false
}

func ValidateJournal(journal Journal) error {
	if journal.Version != 1 || journal.Batch == "" || len(journal.Records) == 0 {
		return fmt.Errorf("journal de fenetres vide ou version invalide")
	}
	seen := map[string]bool{}
	for _, record := range journal.Records {
		window := record.Before
		id, err := strconv.ParseUint(window.ID, 10, strconv.IntSize)
		if err != nil || id == 0 || seen[window.ID] || window.ProcessID == 0 || window.Started == "" || window.Application == "" || window.Blocked != "" {
			return fmt.Errorf("identite de fenetre invalide dans le journal")
		}
		seen[window.ID] = true
		for _, value := range []string{window.DesktopID, record.Target} {
			guid, err := winapi.ParseDesktopID(value)
			if err != nil || winapi.FormatDesktopID(guid) != value {
				return fmt.Errorf("bureau invalide dans le journal")
			}
		}
		switch record.State {
		case "pending", "moving", "applied", "failed", "restoring", "restored":
		default:
			return fmt.Errorf("etat de journal inconnu : %s", record.State)
		}
	}
	return nil
}

func (engine *Engine) PreviewUndo() (Plan, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.current = Plan{}
	journal, err := engine.journals.LoadJournal()
	if err != nil {
		return Plan{}, err
	}
	if err := ValidateJournal(journal); err != nil {
		return Plan{}, err
	}
	inventory, err := engine.backend.Inventory()
	if err != nil {
		return Plan{}, err
	}
	id, err := token()
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{ID: id, Mode: "undo", Desktops: inventory.Desktops, Displays: inventory.Displays, CurrentDesktopID: inventory.CurrentDesktopID, Items: []Item{}}
	for _, record := range journal.Records {
		expected := record.Before
		expected.DesktopID = record.Target
		expected.Pinned = record.Target == winapi.AllDesktopsID
		item := Item{Window: expected, TargetID: record.Before.DesktopID, Source: "undo", Status: "blocked", Explanation: "Mouvement non confirme ou deja restaure ; verifier manuellement"}
		if record.State == "applied" {
			current, found := findWindow(inventory, expected.ID)
			if found && sameWindow(expected, current) && hasDestination(inventory.Desktops, item.TargetID) {
				item.Status, item.Explanation = "pending", "Restaurer le bureau d'origine"
			} else {
				item.Explanation = "Fenetre modifiee, fermee ou bureau d'origine absent"
			}
		}
		plan.Items = append(plan.Items, item)
	}
	engine.current, engine.undo = plan, journal
	engine.undo.Records = append([]Record{}, journal.Records...)
	return clonePlan(plan), nil
}

func selected(item Item, ruleIDs []string) bool {
	if len(ruleIDs) == 0 {
		return true
	}
	for _, id := range item.RuleIDs {
		if slices.Contains(ruleIDs, id) {
			return true
		}
	}
	return false
}

// Apply runs the desktop moves of the plan. With ruleIDs, only windows matched
// by those rules are handled. Report.Layouts carries the geometry to apply next;
// ErrNothing is returned when there is no desktop move (layouts may still exist).
func (engine *Engine) Apply(planID string, ruleIDs ...string) (Report, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	report := Report{Results: []Result{}}
	if planID == "" || engine.current.ID != planID {
		return report, fmt.Errorf("apercu obsolete : actualiser")
	}
	plan := engine.current
	engine.current = Plan{}
	journal := engine.undo
	if plan.Mode == "arrange" {
		for _, layout := range plan.Layouts {
			if len(ruleIDs) == 0 || slices.Contains(ruleIDs, layout.RuleID) || overlaps(ruleIDs, layout.RuleIDs) {
				report.Layouts = append(report.Layouts, layout)
			}
		}
		journal = Journal{Version: 1, Batch: plan.ID, Records: []Record{}}
		for _, item := range plan.Items {
			if item.Status == "pending" && selected(item, ruleIDs) {
				journal.Records = append(journal.Records, Record{Before: item.Window, Target: item.TargetID, State: "pending"})
			}
		}
	}
	if len(journal.Records) == 0 {
		return report, ErrNothing
	}
	if plan.Mode == "arrange" {
		previous, err := engine.journals.LoadJournal()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return report, fmt.Errorf("journal precedent illisible ; le conserver avant toute autre action : %w", err)
		}
		if err == nil {
			if err := ValidateJournal(previous); err != nil {
				return report, err
			}
			for _, record := range previous.Records {
				if record.State == "moving" || record.State == "restoring" {
					report.Recovery = &previous
					return report, fmt.Errorf("journal precedent avec mouvements incertains : verifier les bureaux et archiver ce journal avant un nouveau lot")
				}
			}
		}
	}
	if plan.Mode == "undo" {
		current, err := engine.journals.LoadJournal()
		if err != nil {
			return report, err
		}
		if !sameJournal(current, journal) {
			return report, fmt.Errorf("journal modifie depuis l'apercu : recommencer")
		}
	}
	persist := func() error {
		if err := engine.journals.SaveJournal(journal); err != nil {
			copy := journal
			copy.Records = append([]Record{}, journal.Records...)
			report.Recovery = &copy
			return fmt.Errorf("journalisation interrompue ; ne pas relancer sans verifier les donnees de recuperation : %w", err)
		}
		return nil
	}
	if err := persist(); err != nil {
		return report, err
	}
	for _, item := range plan.Items {
		if plan.Mode == "arrange" && !selected(item, ruleIDs) {
			continue
		}
		if item.Status != "pending" {
			report.Results = append(report.Results, Result{ID: item.Window.ID, Title: item.Window.Title, Status: "ignored", Message: item.Explanation})
			continue
		}
		inventory, err := engine.backend.Inventory()
		if err != nil {
			return report, fmt.Errorf("revalidation interrompue : %w", err)
		}
		current, found := findWindow(inventory, item.Window.ID)
		if !found || !sameWindow(item.Window, current) || !hasDestination(inventory.Desktops, item.TargetID) {
			report.Results = append(report.Results, Result{ID: item.Window.ID, Title: item.Window.Title, Status: "ignored", Message: "Affectation obsolete : actualiser"})
			continue
		}
		recordIndex := -1
		for index, record := range journal.Records {
			if record.Before.ID == item.Window.ID {
				recordIndex = index
				break
			}
		}
		if recordIndex < 0 {
			return report, fmt.Errorf("mouvement sans entree de journal")
		}
		journal.Records[recordIndex].State = "moving"
		if plan.Mode == "undo" {
			journal.Records[recordIndex].State = "restoring"
		}
		if err := persist(); err != nil {
			return report, err
		}
		moveErr := engine.backend.Move(current, item.TargetID)
		if moveErr != nil {
			report.Results = append(report.Results, Result{ID: current.ID, Title: current.Title, Status: "uncertain", Message: moveErr.Error() + " ; verifier l'etat reel"})
		} else {
			after, readErr := engine.backend.Inventory()
			moved, exists := findWindow(after, current.ID)
			expected := current
			expected.DesktopID = item.TargetID
			expected.Pinned = item.TargetID == winapi.AllDesktopsID
			if readErr != nil || !exists || !sameWindow(expected, moved) {
				message := "Mouvement non confirme : verifier l'etat reel"
				if readErr != nil {
					message += " : " + readErr.Error()
				}
				report.Results = append(report.Results, Result{ID: current.ID, Title: current.Title, Status: "uncertain", Message: message})
			} else {
				journal.Records[recordIndex].State = "applied"
				if plan.Mode == "undo" {
					journal.Records[recordIndex].State = "restored"
				}
				report.Results = append(report.Results, Result{ID: current.ID, Title: current.Title, Status: "success", Message: "Bureau de destination verifie"})
			}
		}
		if err := persist(); err != nil {
			return report, err
		}
	}
	return report, nil
}

func sameJournal(left, right Journal) bool {
	if left.Version != right.Version || left.Batch != right.Batch || len(left.Records) != len(right.Records) {
		return false
	}
	for index := range left.Records {
		if left.Records[index] != right.Records[index] {
			return false
		}
	}
	return true
}
