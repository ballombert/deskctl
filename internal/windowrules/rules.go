package windowrules

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"deskctl/internal/winapi"
)

const (
	Version     = 3
	maxDepth    = 4
	maxChildren = 12
)

// Node is a tiling tree element: a group splits its area along Axis between its
// children according to Weight (percent); a window leaf matches windows by regex.
type Node struct {
	Kind        string  `json:"kind"`
	Axis        string  `json:"axis,omitempty"`
	Weight      float64 `json:"weight"`
	Application string  `json:"application,omitempty"`
	Title       string  `json:"title,omitempty"`
	Children    []Node  `json:"children,omitempty"`
}

type Rule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Application string `json:"application"`
	Title       string `json:"title"`
	DesktopID   string `json:"desktopId"`
	DisplayID   string `json:"displayId,omitempty"`
	Layout      *Node  `json:"layout,omitempty"`
	GroupID     string `json:"groupId,omitempty"`
	Zone        *Zone  `json:"zone,omitempty"`
	group       *Group
}

// Zone uses fractions of the monitor work area, independent of resolution.
type Zone struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Group struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Layout Node   `json:"layout"`
	Zone   *Zone  `json:"zone,omitempty"`
}

type Document struct {
	Version int     `json:"version"`
	Rules   []Rule  `json:"rules"`
	Groups  []Group `json:"groups"`
}

type Candidate struct {
	ID          string
	Application string
	Title       string
}

func Empty() Document { return Document{Version: Version, Rules: []Rule{}, Groups: []Group{}} }

func ValidateZone(zone *Zone) error {
	if zone == nil {
		return nil
	}
	for _, value := range []float64{zone.X, zone.Y, zone.Width, zone.Height} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("zone non finie")
		}
	}
	if zone.X < 0 || zone.Y < 0 || zone.Width <= 0 || zone.Height <= 0 ||
		zone.X+zone.Width > 1.000001 || zone.Y+zone.Height > 1.000001 {
		return fmt.Errorf("zone hors de l'ecran (fractions entre 0 et 1)")
	}
	return nil
}

func (zone Zone) Rect(area winapi.Rect) winapi.Rect {
	return winapi.Rect{
		X:      area.X + int32(math.Round(zone.X*float64(area.Width))),
		Y:      area.Y + int32(math.Round(zone.Y*float64(area.Height))),
		Width:  max(1, int32(math.Round(zone.Width*float64(area.Width)))),
		Height: max(1, int32(math.Round(zone.Height*float64(area.Height)))),
	}
}

func (rule Rule) EffectiveLayout() *Node {
	if rule.group != nil {
		return &rule.group.Layout
	}
	return rule.Layout
}

func (rule Rule) EffectiveZone() *Zone {
	if rule.Zone != nil {
		return rule.Zone
	}
	if rule.group != nil {
		return rule.group.Zone
	}
	return nil
}

var patterns sync.Map

func compile(pattern string) (*regexp.Regexp, error) {
	if cached, ok := patterns.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	expression, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	patterns.Store(pattern, expression)
	return expression, nil
}

func matchPattern(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	expression, err := compile(pattern)
	return err == nil && expression.MatchString(value)
}

// Normalize validates a document, migrates version 1 (exact application, title
// substring) to version 2 (case-insensitive regular expressions) and
// normalizes group weights so that each group sums to 100.
func Normalize(doc Document) (Document, error) {
	if doc.Version != 1 && doc.Version != 2 && doc.Version != Version {
		return Document{}, fmt.Errorf("version de regles non prise en charge : %d", doc.Version)
	}
	if doc.Version == Version && doc.Rules == nil {
		return Document{}, fmt.Errorf("liste de regles absente")
	}
	legacy := doc.Version == 1
	result := Empty()
	groupIDs := map[string]bool{}
	for _, group := range doc.Groups {
		group.ID, group.Name = strings.TrimSpace(group.ID), strings.TrimSpace(group.Name)
		if group.ID == "" || group.Name == "" || groupIDs[group.ID] {
			return Document{}, fmt.Errorf("groupe sans nom ou identifiant unique : %q", group.ID)
		}
		groupIDs[group.ID] = true
		layout, err := normalizeNode(group.Layout, 1)
		if err != nil {
			return Document{}, fmt.Errorf("groupe %q : %w", group.Name, err)
		}
		if layout.Kind != "group" {
			return Document{}, fmt.Errorf("groupe %q : racine de disposition invalide", group.Name)
		}
		layout.Weight = 100
		group.Layout = layout
		if err := ValidateZone(group.Zone); err != nil {
			return Document{}, fmt.Errorf("groupe %q : %w", group.Name, err)
		}
		result.Groups = append(result.Groups, group)
	}
	seen := map[string]bool{}
	for _, rule := range doc.Rules {
		rule.group = nil
		rule.ID, rule.Name = strings.TrimSpace(rule.ID), strings.TrimSpace(rule.Name)
		rule.Application, rule.Title = strings.TrimSpace(rule.Application), strings.TrimSpace(rule.Title)
		rule.DisplayID = strings.TrimSpace(rule.DisplayID)
		rule.GroupID = strings.TrimSpace(rule.GroupID)
		if err := ValidateZone(rule.Zone); err != nil {
			return Document{}, fmt.Errorf("regle %q : %w", rule.Name, err)
		}
		if doc.Version < Version && (rule.GroupID != "" || rule.Zone != nil || len(doc.Groups) != 0) {
			return Document{}, fmt.Errorf("groupes et zones exigent la version 3")
		}
		if legacy {
			if rule.Layout != nil || rule.DisplayID != "" {
				return Document{}, fmt.Errorf("regle %q : ecran ou disposition interdits en version 1", rule.Name)
			}
			if rule.Application != "" {
				rule.Application = "^" + regexp.QuoteMeta(rule.Application) + "$"
			}
			rule.Title = regexp.QuoteMeta(rule.Title)
		}
		if rule.ID == "" || seen[rule.ID] || rule.Name == "" {
			return Document{}, fmt.Errorf("regle sans nom, sans identifiant ou identifiant duplique : %q", rule.ID)
		}
		seen[rule.ID] = true
		if rule.Layout != nil {
			if rule.GroupID != "" {
				return Document{}, fmt.Errorf("regle %q : groupe et disposition concurrents", rule.Name)
			}
			if rule.Application != "" || rule.Title != "" {
				return Document{}, fmt.Errorf("regle %q : avec une disposition, les conditions vont dans les fenetres du groupe", rule.Name)
			}
			layout, err := normalizeNode(*rule.Layout, 1)
			if err != nil {
				return Document{}, fmt.Errorf("regle %q : %w", rule.Name, err)
			}
			if layout.Kind != "group" {
				return Document{}, fmt.Errorf("regle %q : la disposition doit commencer par un groupe", rule.Name)
			}
			layout.Weight = 100
			id := "group-" + rule.ID
			for index := 2; groupIDs[id]; index++ {
				id = fmt.Sprintf("group-%s-%d", rule.ID, index)
			}
			groupIDs[id] = true
			result.Groups = append(result.Groups, Group{ID: id, Name: rule.Name, Layout: layout})
			rule.Layout, rule.GroupID = nil, id
		} else if rule.GroupID != "" {
			if rule.Application != "" || rule.Title != "" {
				return Document{}, fmt.Errorf("regle %q : un groupe ne peut pas avoir de condition concurrente", rule.Name)
			}
		} else {
			if rule.Application == "" && rule.Title == "" {
				return Document{}, fmt.Errorf("regle %q : application ou titre obligatoire", rule.Name)
			}
			if err := checkPatterns(rule.Application, rule.Title); err != nil {
				return Document{}, fmt.Errorf("regle %q : %w", rule.Name, err)
			}
		}
		id, err := winapi.ParseDesktopID(rule.DesktopID)
		if err != nil {
			return Document{}, fmt.Errorf("regle %q : %w", rule.Name, err)
		}
		rule.DesktopID = winapi.FormatDesktopID(id)
		result.Rules = append(result.Rules, rule)
	}
	for index := range result.Rules {
		rule := &result.Rules[index]
		if rule.GroupID == "" {
			continue
		}
		for groupIndex := range result.Groups {
			if result.Groups[groupIndex].ID == rule.GroupID {
				rule.group = &result.Groups[groupIndex]
				break
			}
		}
		if rule.group == nil {
			return Document{}, fmt.Errorf("regle %q : groupe absent %q", rule.Name, rule.GroupID)
		}
	}
	return result, nil
}

func checkPatterns(values ...string) error {
	for _, value := range values {
		if value == "" {
			continue
		}
		if len(value) > 512 {
			return fmt.Errorf("expression trop longue")
		}
		if _, err := compile(value); err != nil {
			return fmt.Errorf("expression invalide %q : %w", value, err)
		}
	}
	return nil
}

func normalizeNode(node Node, depth int) (Node, error) {
	node.Application, node.Title = strings.TrimSpace(node.Application), strings.TrimSpace(node.Title)
	if math.IsNaN(node.Weight) || math.IsInf(node.Weight, 0) || node.Weight < 0 {
		return Node{}, fmt.Errorf("taille invalide")
	}
	switch node.Kind {
	case "window":
		if len(node.Children) != 0 || node.Axis != "" {
			return Node{}, fmt.Errorf("une fenetre ne contient pas d'autres elements")
		}
		if node.Application == "" && node.Title == "" {
			return Node{}, fmt.Errorf("chaque fenetre du groupe exige une application ou un titre")
		}
		return node, checkPatterns(node.Application, node.Title)
	case "group":
		if node.Axis != "h" && node.Axis != "v" {
			return Node{}, fmt.Errorf("axe de groupe invalide %q (h ou v)", node.Axis)
		}
		if node.Application != "" || node.Title != "" {
			return Node{}, fmt.Errorf("un groupe n'a pas de condition propre")
		}
		if depth > maxDepth {
			return Node{}, fmt.Errorf("imbrication limitee a %d niveaux", maxDepth)
		}
		if len(node.Children) == 0 || len(node.Children) > maxChildren {
			return Node{}, fmt.Errorf("un groupe contient entre 1 et %d elements", maxChildren)
		}
		children := make([]Node, 0, len(node.Children))
		total := 0.0
		for _, child := range node.Children {
			normalized, err := normalizeNode(child, depth+1)
			if err != nil {
				return Node{}, err
			}
			total += normalized.Weight
			children = append(children, normalized)
		}
		for index := range children {
			if total == 0 {
				children[index].Weight = 100 / float64(len(children))
			} else {
				children[index].Weight = children[index].Weight * 100 / total
			}
			children[index].Weight = math.Round(children[index].Weight*100) / 100
		}
		node.Children = children
		return node, nil
	default:
		return Node{}, fmt.Errorf("type d'element inconnu %q", node.Kind)
	}
}

func (node Node) matches(application, title string) bool {
	return node.Kind == "window" && matchPattern(node.Application, application) && matchPattern(node.Title, title)
}

// HasGeometry reports whether the rule positions windows on a screen.
func (rule Rule) HasGeometry() bool { return rule.EffectiveLayout() != nil || rule.DisplayID != "" }

func (rule Rule) Matches(application, title string) bool {
	if !rule.Enabled {
		return false
	}
	layout := rule.EffectiveLayout()
	if layout == nil {
		return matchPattern(rule.Application, application) && matchPattern(rule.Title, title)
	}
	_, found := layout.leafFor(application, title, "")
	return found
}

func (node Node) leafFor(application, title, path string) (string, bool) {
	if node.Kind == "window" {
		return path, node.matches(application, title)
	}
	for index, child := range node.Children {
		if found, ok := child.leafFor(application, title, childPath(path, index)); ok {
			return found, true
		}
	}
	return "", false
}

func childPath(parent string, index int) string {
	if parent == "" {
		return strconv.Itoa(index)
	}
	return parent + "." + strconv.Itoa(index)
}

// Slots assigns every matching window to the first leaf (depth-first order) it matches.
func (rule Rule) Slots(windows []Candidate) map[string][]string {
	slots := map[string][]string{}
	layout := rule.EffectiveLayout()
	if layout == nil {
		return slots
	}
	for _, window := range windows {
		if path, ok := layout.leafFor(window.Application, window.Title, ""); ok {
			slots[path] = append(slots[path], window.ID)
		}
	}
	return slots
}
