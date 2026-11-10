package grouptracking

import (
	"errors"
	"testing"

	"deskctl/internal/winapi"
)

type fakeBackend struct {
	windows   map[string]Member
	areas     []winapi.Rect
	saveErr   error
	placeErr  map[string]error
	ignore    map[string]bool
	saves     int
	placed    []string
	afterSave []Member
}

func (backend *fakeBackend) Read(id string) (Member, error) {
	member, ok := backend.windows[id]
	if !ok {
		return Member{}, errors.New("fenetre fermee")
	}
	return member, nil
}

func (backend *fakeBackend) Areas() ([]winapi.Rect, error) { return backend.areas, nil }

func (backend *fakeBackend) Place(member Member, target winapi.Rect) error {
	if err := backend.placeErr[member.ID]; err != nil {
		return err
	}
	backend.placed = append(backend.placed, member.ID)
	if backend.ignore[member.ID] {
		return nil
	}
	live := backend.windows[member.ID]
	if target.Width != live.Rect.Width || target.Height != live.Rect.Height {
		panic("peer resized")
	}
	live.Rect = target
	backend.windows[member.ID] = live
	return nil
}

func (backend *fakeBackend) Save(before, after []Member) error {
	backend.saves++
	backend.afterSave = after
	return backend.saveErr
}

func member(id string, x, y int32) Member {
	return Member{ID: id, ProcessID: 100, Started: "t0", Application: "app", Class: "cls", DesktopID: "d1",
		Rect: winapi.Rect{X: x, Y: y, Width: 400, Height: 300}}
}

func setup(t *testing.T) (*Tracker, *fakeBackend) {
	t.Helper()
	backend := &fakeBackend{
		windows: map[string]Member{"1": member("1", 0, 0), "2": member("2", 500, 0), "3": member("3", 0, 400)},
		areas:   []winapi.Rect{{X: 0, Y: 0, Width: 1920, Height: 1040}, {X: -1920, Y: 0, Width: 1920, Height: 1040}},
	}
	tracker := New(backend)
	group := Group{ID: "g", Members: []Member{backend.windows["1"], backend.windows["2"], backend.windows["3"]}}
	if err := tracker.SetGroups([]Group{group}); err != nil {
		t.Fatal(err)
	}
	if len(backend.placed) != 0 || backend.saves != 0 {
		t.Fatal("SetGroups must not apply anything")
	}
	return tracker, backend
}

func drag(backend *fakeBackend, id string, x, y, width, height int32) {
	live := backend.windows[id]
	live.Rect = winapi.Rect{X: x, Y: y, Width: width, Height: height}
	backend.windows[id] = live
}

func TestEndTranslatesPeersOnce(t *testing.T) {
	tracker, backend := setup(t)
	if err := tracker.Begin("1"); err != nil {
		t.Fatal(err)
	}
	drag(backend, "1", 100, 50, 400, 300)
	groupID, changed, err := tracker.End("1")
	if err != nil || !changed || groupID != "g" {
		t.Fatalf("End = %q %v %v", groupID, changed, err)
	}
	if got := backend.windows["2"].Rect; got != (winapi.Rect{X: 600, Y: 50, Width: 400, Height: 300}) {
		t.Fatalf("peer 2 = %+v", got)
	}
	if got := backend.windows["3"].Rect; got != (winapi.Rect{X: 100, Y: 450, Width: 400, Height: 300}) {
		t.Fatalf("peer 3 = %+v", got)
	}
	if len(backend.placed) != 2 || backend.saves != 1 {
		t.Fatalf("placed %v saves %d", backend.placed, backend.saves)
	}
	if _, changed, err := tracker.End("1"); changed || err != nil {
		t.Fatalf("second End must be ignored: %v %v", changed, err)
	}
	if len(backend.placed) != 2 {
		t.Fatal("peers placed twice")
	}
}

func TestEndMovesToOtherDisplayWithNegativeCoordinates(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	drag(backend, "1", -1900, 10, 400, 300)
	if _, changed, err := tracker.End("1"); err != nil || !changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if got := backend.windows["2"].Rect.X; got != -1400 {
		t.Fatalf("peer 2 X = %d", got)
	}
}

func TestEndIgnoresResizeAndUnmatched(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	if _, changed, err := tracker.End("2"); changed || err != nil {
		t.Fatalf("unmatched End: %v %v", changed, err)
	}
	tracker.Begin("1")
	drag(backend, "1", 0, 0, 1920, 1040)
	groupID, changed, err := tracker.End("1")
	if err != nil || changed || groupID != "g" || backend.saves != 0 || len(backend.placed) != 0 {
		t.Fatalf("resize must be ignored: %q %v %v", groupID, changed, err)
	}
}

func TestEndToleratesDPIRounding(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	drag(backend, "1", 10, 10, 402, 299)
	if _, changed, err := tracker.End("1"); err != nil || !changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if got := backend.windows["2"].Rect; got.Width != 400 || got.Height != 300 {
		t.Fatalf("peer resized: %+v", got)
	}
}

func TestSetGroupsRejectsDuplicateMembership(t *testing.T) {
	tracker := New(&fakeBackend{})
	a, b := member("1", 0, 0), member("2", 0, 0)
	err := tracker.SetGroups([]Group{{ID: "g1", Members: []Member{a, b}}, {ID: "g2", Members: []Member{b}}})
	if !errors.Is(err, ErrInvalidGroups) {
		t.Fatalf("multiple groups: %v", err)
	}
	if err := tracker.SetGroups([]Group{{ID: "g1", Members: []Member{a, a}}}); !errors.Is(err, ErrInvalidGroups) {
		t.Fatalf("duplicate member: %v", err)
	}
	invalid := a
	invalid.Started = ""
	if err := tracker.SetGroups([]Group{{ID: "g1", Members: []Member{invalid}}}); !errors.Is(err, ErrInvalidGroups) {
		t.Fatalf("invalid member: %v", err)
	}
}

func TestBeginRejectsReusedWindowID(t *testing.T) {
	tracker, backend := setup(t)
	reused := backend.windows["2"]
	reused.ProcessID = 999
	reused.Started = "t1"
	backend.windows["2"] = reused
	if err := tracker.Begin("1"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("Begin = %v", err)
	}
	drag(backend, "1", 100, 0, 400, 300)
	if _, changed, err := tracker.End("1"); changed || err != nil || len(backend.placed) != 0 {
		t.Fatalf("End without snapshot: %v %v", changed, err)
	}
}

func TestEndRejectsSourceReusedDuringDrag(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	reused := backend.windows["1"]
	reused.Started = "t9"
	reused.Rect.X = 100
	backend.windows["1"] = reused
	if _, _, err := tracker.End("1"); !errors.Is(err, ErrIdentityMismatch) || len(backend.placed) != 0 {
		t.Fatalf("End = %v", err)
	}
}

func TestEndRejectsPeerMovedDuringDrag(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	drag(backend, "2", 510, 0, 400, 300)
	drag(backend, "1", 100, 0, 400, 300)
	if _, changed, err := tracker.End("1"); !errors.Is(err, ErrPeerChanged) || changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if backend.saves != 0 || len(backend.placed) != 0 {
		t.Fatal("nothing must be saved or placed")
	}
}

func TestEndRejectsTargetOutsideSingleWorkArea(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	drag(backend, "1", 1500, 0, 400, 300)
	if _, changed, err := tracker.End("1"); !errors.Is(err, ErrOutsideWorkArea) || changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if backend.saves != 0 || len(backend.placed) != 0 {
		t.Fatal("nothing must be saved or placed")
	}
}

func TestEndJournalFailurePlacesNothing(t *testing.T) {
	tracker, backend := setup(t)
	backend.saveErr = errors.New("disque plein")
	tracker.Begin("1")
	drag(backend, "1", 100, 0, 400, 300)
	if _, changed, err := tracker.End("1"); !errors.Is(err, ErrJournal) || changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if len(backend.placed) != 0 {
		t.Fatal("peers placed despite journal failure")
	}
}

func TestEndSavesBeforeAndAfter(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	drag(backend, "1", 100, 0, 400, 300)
	tracker.End("1")
	if len(backend.afterSave) != 3 || backend.afterSave[0].Rect.X != 100 || backend.afterSave[1].Rect.X != 600 {
		t.Fatalf("after = %+v", backend.afterSave)
	}
}

func TestEndReportsPartialPlacement(t *testing.T) {
	tracker, backend := setup(t)
	backend.placeErr = map[string]error{"3": errors.New("acces refuse")}
	tracker.Begin("1")
	drag(backend, "1", 100, 0, 400, 300)
	_, changed, err := tracker.End("1")
	var partial *PlacementError
	if !errors.As(err, &partial) || !errors.Is(err, ErrPartialPlacement) || !changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if partial.MemberID != "3" || len(partial.Placed) != 1 || partial.Placed[0] != "2" {
		t.Fatalf("partial = %+v", partial)
	}
	if backend.windows["2"].Rect.X != 600 {
		t.Fatal("no blind rollback expected")
	}
}

func TestEndReportsUnconfirmedPlacement(t *testing.T) {
	tracker, backend := setup(t)
	backend.ignore = map[string]bool{"2": true}
	tracker.Begin("1")
	drag(backend, "1", 100, 0, 400, 300)
	_, changed, err := tracker.End("1")
	var uncertain *UncertainError
	if !errors.As(err, &uncertain) || !errors.Is(err, ErrUncertainPlacement) || !changed {
		t.Fatalf("End = %v %v", changed, err)
	}
	if len(uncertain.MemberIDs) != 1 || uncertain.MemberIDs[0] != "2" {
		t.Fatalf("uncertain = %+v", uncertain)
	}
}

func TestEndRejectsDesktopChange(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	moved := backend.windows["1"]
	moved.DesktopID = "d2"
	moved.Rect.X = 100
	backend.windows["1"] = moved
	if _, _, err := tracker.End("1"); !errors.Is(err, ErrIncoherentDesktop) || len(backend.placed) != 0 {
		t.Fatalf("End = %v", err)
	}
}

func TestCancelClearsSnapshot(t *testing.T) {
	tracker, backend := setup(t)
	tracker.Begin("1")
	tracker.Cancel()
	drag(backend, "1", 100, 0, 400, 300)
	if _, changed, err := tracker.End("1"); changed || err != nil {
		t.Fatalf("End after Cancel = %v %v", changed, err)
	}
}
