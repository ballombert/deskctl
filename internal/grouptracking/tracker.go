package grouptracking

import (
	"fmt"

	"deskctl/internal/winapi"
)

// Tracker follows one drag at a time. Calls must be serialized by the caller.
type Tracker struct {
	backend  Backend
	groups   []Group
	byWindow map[string]int
	active   *snapshot
}

type snapshot struct {
	groupID  string
	sourceID string
	members  []Member
}

// New creates a tracker with no groups.
func New(backend Backend) *Tracker {
	return &Tracker{backend: backend, byWindow: map[string]int{}}
}

// SetGroups replaces the enabled groups after validation. It never places windows and cancels any active drag.
func (tracker *Tracker) SetGroups(groups []Group) error {
	byWindow := map[string]int{}
	groupIDs := map[string]bool{}
	copied := make([]Group, 0, len(groups))
	for index, group := range groups {
		if group.ID == "" {
			return fmt.Errorf("%w : groupe %d sans identifiant", ErrInvalidGroups, index+1)
		}
		if groupIDs[group.ID] {
			return fmt.Errorf("%w : groupe %s en double", ErrInvalidGroups, group.ID)
		}
		groupIDs[group.ID] = true
		if len(group.Members) == 0 {
			return fmt.Errorf("%w : groupe %s vide", ErrInvalidGroups, group.ID)
		}
		for _, member := range group.Members {
			if member.ID == "" || member.ProcessID == 0 || member.Started == "" || member.Application == "" || member.Class == "" {
				return fmt.Errorf("%w : groupe %s, membre %q sans identite stable", ErrInvalidGroups, group.ID, member.ID)
			}
			if !member.Pinned && member.DesktopID == "" {
				return fmt.Errorf("%w : groupe %s, membre %s sans bureau virtuel", ErrInvalidGroups, group.ID, member.ID)
			}
			if previous, exists := byWindow[member.ID]; exists {
				if previous == len(copied) {
					return fmt.Errorf("%w : fenetre %s en double dans le groupe %s", ErrInvalidGroups, member.ID, group.ID)
				}
				return fmt.Errorf("%w : fenetre %s dans les groupes %s et %s", ErrInvalidGroups, member.ID, copied[previous].ID, group.ID)
			}
			byWindow[member.ID] = len(copied)
		}
		copied = append(copied, Group{ID: group.ID, Members: append([]Member(nil), group.Members...)})
	}
	tracker.groups = copied
	tracker.byWindow = byWindow
	tracker.active = nil
	return nil
}

// Begin snapshots the group of windowID when a drag starts. Windows outside any group are ignored.
func (tracker *Tracker) Begin(windowID string) error {
	tracker.active = nil
	index, ok := tracker.byWindow[windowID]
	if !ok {
		return nil
	}
	group := tracker.groups[index]
	members := make([]Member, 0, len(group.Members))
	for _, configured := range group.Members {
		live, err := tracker.readMatching(group.ID, configured)
		if err != nil {
			return err
		}
		members = append(members, live)
	}
	if err := coherentDesktops(group.ID, members); err != nil {
		return err
	}
	tracker.active = &snapshot{groupID: group.ID, sourceID: windowID, members: members}
	return nil
}

// Cancel forgets the active drag snapshot.
func (tracker *Tracker) Cancel() {
	tracker.active = nil
}

// End translates the peers of the dragged window once. changed is true when peers were placed.
// Unmatched events and source resizes are ignored without error. The snapshot is always cleared.
func (tracker *Tracker) End(windowID string) (groupID string, changed bool, err error) {
	active := tracker.active
	if active == nil || active.sourceID != windowID {
		return "", false, nil
	}
	tracker.active = nil
	groupID = active.groupID

	var sourceBefore Member
	peers := make([]Member, 0, len(active.members)-1)
	for _, member := range active.members {
		if member.ID == windowID {
			sourceBefore = member
		} else {
			peers = append(peers, member)
		}
	}
	sourceAfter, err := tracker.backend.Read(windowID)
	if err != nil {
		return groupID, false, fmt.Errorf("groupe %s, lecture de la fenetre %s : %w", groupID, windowID, err)
	}
	if !SameIdentity(sourceBefore, sourceAfter) {
		return groupID, false, fmt.Errorf("%w : groupe %s, fenetre %s", ErrIdentityMismatch, groupID, windowID)
	}
	if !SameDesktop(sourceBefore, sourceAfter) {
		return groupID, false, fmt.Errorf("%w : groupe %s, fenetre %s a change de bureau", ErrIncoherentDesktop, groupID, windowID)
	}
	if !ValidRect(sourceAfter.Rect) || !SameSize(sourceBefore.Rect, sourceAfter.Rect, SizeTolerance) {
		return groupID, false, nil
	}
	dx := int64(sourceAfter.Rect.X) - int64(sourceBefore.Rect.X)
	dy := int64(sourceAfter.Rect.Y) - int64(sourceBefore.Rect.Y)
	if (dx == 0 && dy == 0) || len(peers) == 0 {
		return groupID, false, nil
	}

	for _, peer := range peers {
		live, err := tracker.backend.Read(peer.ID)
		if err != nil {
			return groupID, false, fmt.Errorf("%w : groupe %s, fenetre %s : %v", ErrPeerChanged, groupID, peer.ID, err)
		}
		if !SameIdentity(peer, live) || !SameDesktop(peer, live) || live.Rect != peer.Rect {
			return groupID, false, fmt.Errorf("%w : groupe %s, fenetre %s", ErrPeerChanged, groupID, peer.ID)
		}
	}

	targets := make([]winapi.Rect, len(peers))
	for index, peer := range peers {
		target, err := Translate(peer.Rect, dx, dy)
		if err != nil {
			return groupID, false, fmt.Errorf("groupe %s, fenetre %s : %w", groupID, peer.ID, err)
		}
		targets[index] = target
	}
	areas, err := tracker.backend.Areas()
	if err != nil {
		return groupID, false, fmt.Errorf("groupe %s, zones de travail : %w", groupID, err)
	}
	if !fitsOneArea(areas, append([]winapi.Rect{sourceAfter.Rect}, targets...)) {
		return groupID, false, fmt.Errorf("%w : groupe %s", ErrOutsideWorkArea, groupID)
	}

	before := make([]Member, 0, len(active.members))
	after := make([]Member, 0, len(active.members))
	before = append(before, sourceBefore)
	after = append(after, sourceAfter)
	for index, peer := range peers {
		before = append(before, peer)
		moved := peer
		moved.Rect = targets[index]
		after = append(after, moved)
	}
	if err := tracker.backend.Save(before, after); err != nil {
		return groupID, false, fmt.Errorf("%w : groupe %s : %v", ErrJournal, groupID, err)
	}

	placed := make([]string, 0, len(peers))
	for index, peer := range peers {
		if err := tracker.backend.Place(peer, targets[index]); err != nil {
			pending := make([]string, 0, len(peers)-index-1)
			for _, rest := range peers[index+1:] {
				pending = append(pending, rest.ID)
			}
			return groupID, len(placed) > 0, &PlacementError{GroupID: groupID, MemberID: peer.ID, Placed: placed, Pending: pending, Err: err}
		}
		placed = append(placed, peer.ID)
	}

	var uncertain []string
	var firstErr error
	for index, peer := range peers {
		live, err := tracker.backend.Read(peer.ID)
		if err != nil || !SameIdentity(peer, live) || !SameRect(live.Rect, targets[index], SizeTolerance) {
			uncertain = append(uncertain, peer.ID)
			if firstErr == nil && err != nil {
				firstErr = err
			}
		}
	}
	if len(uncertain) > 0 {
		return groupID, true, &UncertainError{GroupID: groupID, MemberIDs: uncertain, Err: firstErr}
	}
	return groupID, true, nil
}

func (tracker *Tracker) readMatching(groupID string, configured Member) (Member, error) {
	live, err := tracker.backend.Read(configured.ID)
	if err != nil {
		return Member{}, fmt.Errorf("groupe %s, lecture de la fenetre %s : %w", groupID, configured.ID, err)
	}
	if !SameIdentity(configured, live) {
		return Member{}, fmt.Errorf("%w : groupe %s, fenetre %s", ErrIdentityMismatch, groupID, configured.ID)
	}
	if !SameDesktop(configured, live) {
		return Member{}, fmt.Errorf("%w : groupe %s, fenetre %s", ErrIncoherentDesktop, groupID, configured.ID)
	}
	if !ValidRect(live.Rect) {
		return Member{}, fmt.Errorf("%w : groupe %s, fenetre %s", ErrInvalidGeometry, groupID, configured.ID)
	}
	return live, nil
}

// coherentDesktops requires every unpinned member to live on the same virtual desktop.
func coherentDesktops(groupID string, members []Member) error {
	desktop := ""
	for _, member := range members {
		if member.Pinned {
			continue
		}
		if desktop == "" {
			desktop = member.DesktopID
		} else if member.DesktopID != desktop {
			return fmt.Errorf("%w : groupe %s, fenetre %s", ErrIncoherentDesktop, groupID, member.ID)
		}
	}
	return nil
}

func fitsOneArea(areas []winapi.Rect, rects []winapi.Rect) bool {
	for _, area := range areas {
		if !ValidRect(area) {
			continue
		}
		fits := true
		for _, r := range rects {
			if !Contains(area, r) {
				fits = false
				break
			}
		}
		if fits {
			return true
		}
	}
	return false
}
