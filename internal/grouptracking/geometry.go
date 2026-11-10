// Package grouptracking translates the peers of a window group once a user drag of one member ends.
package grouptracking

import (
	"errors"
	"fmt"
	"strings"

	"deskctl/internal/winapi"
)

// SizeTolerance is the maximum width/height difference (DPI rounding) still treated as a pure move,
// and the maximum position error accepted when confirming a placement.
const SizeTolerance = 2

var (
	ErrInvalidGroups      = errors.New("groupes invalides")
	ErrIdentityMismatch   = errors.New("identite de fenetre differente")
	ErrIncoherentDesktop  = errors.New("bureau virtuel ou epinglage incoherent")
	ErrInvalidGeometry    = errors.New("geometrie de fenetre invalide")
	ErrPeerChanged        = errors.New("fenetre du groupe modifiee pendant le deplacement")
	ErrOutsideWorkArea    = errors.New("groupe hors d'une zone de travail unique")
	ErrJournal            = errors.New("journal du deplacement de groupe impossible")
	ErrPartialPlacement   = errors.New("deplacement de groupe partiel")
	ErrUncertainPlacement = errors.New("placement du groupe non confirme")
)

// Member is a window of a group with its stable identity and current placement.
type Member struct {
	ID          string
	ProcessID   uint32
	Started     string
	Application string
	Class       string
	DesktopID   string
	Pinned      bool
	Rect        winapi.Rect
}

// Group is an enabled set of windows that move together.
type Group struct {
	ID      string
	Members []Member
}

// Backend reads and places real windows. Read returns the live identity, desktop, pin and visible rect.
type Backend interface {
	Read(id string) (Member, error)
	Areas() ([]winapi.Rect, error)
	Place(member Member, target winapi.Rect) error
	Save(before, after []Member) error
}

// PlacementError reports a peer placement failure; Placed peers were already moved, Pending were not attempted.
// No rollback is performed.
type PlacementError struct {
	GroupID  string
	MemberID string
	Placed   []string
	Pending  []string
	Err      error
}

func (err *PlacementError) Error() string {
	return fmt.Sprintf("%s : groupe %s, fenetre %s : %v (deplacees : %s ; non tentees : %s)",
		ErrPartialPlacement, err.GroupID, err.MemberID, err.Err, joinIDs(err.Placed), joinIDs(err.Pending))
}

func (err *PlacementError) Unwrap() []error { return []error{ErrPartialPlacement, err.Err} }

// UncertainError reports peers whose final rectangle could not be confirmed after placement.
type UncertainError struct {
	GroupID   string
	MemberIDs []string
	Err       error
}

func (err *UncertainError) Error() string {
	detail := ""
	if err.Err != nil {
		detail = " : " + err.Err.Error()
	}
	return fmt.Sprintf("%s : groupe %s, fenetres %s%s", ErrUncertainPlacement, err.GroupID, joinIDs(err.MemberIDs), detail)
}

func (err *UncertainError) Unwrap() []error {
	if err.Err == nil {
		return []error{ErrUncertainPlacement}
	}
	return []error{ErrUncertainPlacement, err.Err}
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return "aucune"
	}
	return strings.Join(ids, ", ")
}

// SameIdentity compares the stable identity of two windows (HWND, PID, process start, application, class).
func SameIdentity(a, b Member) bool {
	return a.ID == b.ID && a.ProcessID == b.ProcessID && a.Started == b.Started &&
		strings.EqualFold(a.Application, b.Application) && a.Class == b.Class
}

// SameDesktop compares virtual desktop membership; the desktop ID is ignored for pinned windows.
func SameDesktop(a, b Member) bool {
	if a.Pinned || b.Pinned {
		return a.Pinned == b.Pinned
	}
	return a.DesktopID == b.DesktopID
}

// SameRect reports whether two rectangles differ by at most tolerance on every field.
func SameRect(a, b winapi.Rect, tolerance int32) bool {
	return within(a.X, b.X, tolerance) && within(a.Y, b.Y, tolerance) &&
		within(a.Width, b.Width, tolerance) && within(a.Height, b.Height, tolerance)
}

// SameSize reports whether two rectangles have the same size within tolerance.
func SameSize(a, b winapi.Rect, tolerance int32) bool {
	return within(a.Width, b.Width, tolerance) && within(a.Height, b.Height, tolerance)
}

// ValidRect reports a non-empty rectangle.
func ValidRect(r winapi.Rect) bool {
	return r.Width > 0 && r.Height > 0
}

// Contains reports whether area fully contains r; negative coordinates are allowed.
func Contains(area, r winapi.Rect) bool {
	return int64(r.X) >= int64(area.X) && int64(r.Y) >= int64(area.Y) &&
		int64(r.X)+int64(r.Width) <= int64(area.X)+int64(area.Width) &&
		int64(r.Y)+int64(r.Height) <= int64(area.Y)+int64(area.Height)
}

// Translate shifts r by (dx, dy) without resizing it.
func Translate(r winapi.Rect, dx, dy int64) (winapi.Rect, error) {
	x, y := int64(r.X)+dx, int64(r.Y)+dy
	if x < minInt32 || x > maxInt32 || y < minInt32 || y > maxInt32 ||
		x+int64(r.Width) > maxInt32 || y+int64(r.Height) > maxInt32 {
		return winapi.Rect{}, fmt.Errorf("%w : translation hors limites", ErrInvalidGeometry)
	}
	return winapi.Rect{X: int32(x), Y: int32(y), Width: r.Width, Height: r.Height}, nil
}

const (
	minInt32 = -1 << 31
	maxInt32 = 1<<31 - 1
)

func within(a, b, tolerance int32) bool {
	diff := int64(a) - int64(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= int64(tolerance)
}
