package tiling

import (
	"errors"
	"strings"
	"testing"
	"time"

	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
)

type fakeDesktop struct {
	rects    map[string]winapi.Rect
	refuse   map[string]bool
	stubborn map[string]bool
	floating map[string]bool
	focused  string
}

func (d *fakeDesktop) Rect(id string) (winapi.Rect, error) { return d.rects[id], nil }
func (d *fakeDesktop) SetRect(id string, rect winapi.Rect) error {
	if d.refuse[id] {
		return errors.New("acces refuse")
	}
	if !d.stubborn[id] || d.floating[id] {
		d.rects[id] = rect
	}
	return nil
}
func (d *fakeDesktop) Focus(id string) error { d.focused = id; return nil }
func (d *fakeDesktop) Wait(time.Duration)    {}

type fakeFancy struct {
	running bool
	desktop *fakeDesktop
	actions []string
	tile    func(*fakeDesktop)
}

func (f *fakeFancy) Running() bool { return f.running }
func (f *fakeFancy) Action(action string) error {
	f.actions = append(f.actions, action+"@"+f.desktop.focused)
	if action == "ToggleFloatingMode" {
		f.desktop.floating[f.desktop.focused] = !f.desktop.floating[f.desktop.focused]
	}
	if f.tile != nil {
		f.tile(f.desktop)
	}
	return nil
}

func job() Job {
	node := windowrules.Node{Kind: "group", Axis: "h", Children: []windowrules.Node{
		{Kind: "window", Weight: 75, Application: "a"}, {Kind: "window", Weight: 25, Application: "b"},
	}}
	return Job{Display: winapi.Rect{Width: 1200, Height: 800}, FancyIndex: 2, OnCurrentDesktop: true,
		Layout: windoworganizer.LayoutPlan{Layout: &node, Slots: []windoworganizer.Slot{
			{WindowID: "B", Path: "1", Rect: winapi.Rect{X: 900, Width: 300, Height: 800}},
			{WindowID: "A", Path: "0", Rect: winapi.Rect{Width: 900, Height: 800}},
		}}}
}

func newDesktop() *fakeDesktop {
	return &fakeDesktop{rects: map[string]winapi.Rect{}, refuse: map[string]bool{}, stubborn: map[string]bool{}, floating: map[string]bool{}}
}

func TestNativeFloatsWindowsThatFancyWMKeepsTiling(t *testing.T) {
	desktop := newDesktop()
	desktop.stubborn["A"] = true
	desktop.refuse["B"] = true
	fancy := &fakeFancy{running: true, desktop: desktop}
	outcomes := Native(desktop, fancy, job())
	if outcomes[1].Status != "success" || !strings.Contains(outcomes[1].Message, "flottant") || !desktop.floating["A"] {
		t.Fatal("stubborn window must be floated then placed", outcomes)
	}
	if outcomes[0].Status != "failed" {
		t.Fatal("refused window must be reported", outcomes)
	}
}

func TestAutoUsesFancyWMWhenResultMatches(t *testing.T) {
	desktop := newDesktop()
	fancy := &fakeFancy{running: true, desktop: desktop, tile: func(d *fakeDesktop) {
		d.rects["A"] = winapi.Rect{Width: 880, Height: 790}
		d.rects["B"] = winapi.Rect{X: 890, Width: 310, Height: 790}
	}}
	outcomes := Apply("auto", desktop, fancy, job())
	if outcomes[0].Engine != "fancywm" || outcomes[0].Status != "success" {
		t.Fatal("FancyWM result within tolerance must be kept", outcomes)
	}
	joined := strings.Join(fancy.actions, " ")
	if !strings.HasPrefix(joined, "MoveToDisplay2@A MoveToDisplay2@B CreateHorizontalPanel@A MoveLeft@B IncreaseWidth@A") {
		t.Fatal("unexpected FancyWM sequence", joined)
	}
}

func TestAutoFallsBackToNative(t *testing.T) {
	for name, configure := range map[string]func(*Job, *fakeFancy){
		"not running":   func(_ *Job, f *fakeFancy) { f.running = false },
		"other desktop": func(j *Job, _ *fakeFancy) { j.OnCurrentDesktop = false },
		"no index":      func(j *Job, _ *fakeFancy) { j.FancyIndex = 0 },
		"wrong result": func(_ *Job, f *fakeFancy) {
			f.tile = func(d *fakeDesktop) { d.rects["A"] = winapi.Rect{Width: 600, Height: 800} }
		},
	} {
		desktop := newDesktop()
		fancy := &fakeFancy{running: true, desktop: desktop}
		current := job()
		configure(&current, fancy)
		outcomes := Apply("auto", desktop, fancy, current)
		if outcomes[0].Engine != "native" || outcomes[0].Status != "success" || desktop.rects["A"] != current.Layout.Slots[1].Rect {
			t.Fatal(name, "must fall back to exact native placement", outcomes)
		}

	}
	desktop := newDesktop()
	if outcomes := Apply("fancywm", desktop, &fakeFancy{desktop: desktop}, job()); outcomes[0].Status != "failed" {
		t.Fatal("strict FancyWM mode must not fall back")
	}
}

func TestPartialGroupZoneUsesNativeEvenWithFancyWMAvailable(t *testing.T) {
	desktop := newDesktop()
	fancy := &fakeFancy{running: true, desktop: desktop}
	current := job()
	current.Layout.Bounds = winapi.Rect{X: 100, Y: 50, Width: 900, Height: 600}
	current.Layout.Slots[0].Rect = winapi.Rect{X: 775, Y: 50, Width: 225, Height: 600}
	current.Layout.Slots[1].Rect = winapi.Rect{X: 100, Y: 50, Width: 675, Height: 600}
	outcomes := Apply("fancywm", desktop, fancy, current)
	if len(fancy.actions) != 0 || outcomes[0].Engine != "native" || outcomes[0].Status != "success" ||
		desktop.rects["A"] != current.Layout.Slots[1].Rect || !strings.Contains(outcomes[0].Message, "zone") {
		t.Fatal("partial zones require exact native positioning", outcomes, fancy.actions)
	}
}
