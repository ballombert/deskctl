package gui

import (
	"fmt"
	"strconv"
	"time"

	"deskctl/internal/grouptracking"
	"deskctl/internal/tiling"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowstore"
)

type checkedDesktop struct {
	expected map[string]grouptracking.Member
	native   tiling.WindowsDesktop
}

func checkedJob(job tiling.Job, plan windoworganizer.Plan) (checkedDesktop, error) {
	desktop := checkedDesktop{expected: map[string]grouptracking.Member{}}
	for _, slot := range job.Layout.Slots {
		found := false
		for _, item := range plan.Items {
			if item.Window.ID != slot.WindowID {
				continue
			}
			member := memberOf(item.Window)
			member.DesktopID, member.Pinned = job.Layout.DesktopID, job.Layout.Pinned
			actual, err := (trackingBackend{}).Read(member.ID)
			if err != nil {
				return desktop, err
			}
			if !sameMember(member, actual) || actual.Rect != slot.Before {
				return desktop, fmt.Errorf("fenetre %s modifiee depuis l'apercu : actualiser", slot.Title)
			}
			desktop.expected[member.ID] = member
			found = true
			break
		}
		if !found {
			return desktop, fmt.Errorf("fenetre absente du plan")
		}
	}
	return desktop, nil
}

func (desktop checkedDesktop) check(id string) error {
	expected, ok := desktop.expected[id]
	if !ok {
		return fmt.Errorf("fenetre hors du groupe")
	}
	actual, err := (trackingBackend{}).Read(id)
	if err != nil {
		return err
	}
	if !sameMember(expected, actual) {
		return fmt.Errorf("identite de fenetre modifiee avant le placement")
	}
	return nil
}

func (desktop checkedDesktop) Rect(id string) (winapi.Rect, error) {
	if err := desktop.check(id); err != nil {
		return winapi.Rect{}, err
	}
	return desktop.native.Rect(id)
}

func (desktop checkedDesktop) SetRect(id string, rect winapi.Rect) error {
	if err := desktop.check(id); err != nil {
		return err
	}
	return desktop.native.SetRect(id, rect)
}

func (desktop checkedDesktop) Focus(id string) error {
	if err := desktop.check(id); err != nil {
		return err
	}
	return desktop.native.Focus(id)
}

func (checkedDesktop) Wait(duration time.Duration) { time.Sleep(duration) }

func restoreRecord(record windowstore.GeometryRecord, version int) windoworganizer.Result {
	outcome := windoworganizer.Result{ID: record.WindowID, Title: record.Title, Kind: "layout",
		Engine: "native", Status: "ignored", Message: "Fenetre fermee ou remplacee"}
	actual, err := (trackingBackend{}).Read(record.WindowID)
	if err != nil || actual.ProcessID != record.ProcessID || actual.Started != record.Started {
		return outcome
	}
	if version >= 2 {
		expected := grouptracking.Member{ID: record.WindowID, ProcessID: record.ProcessID, Started: record.Started,
			Application: record.Application, Class: record.Class, DesktopID: record.DesktopID, Pinned: record.Pinned}
		if !sameMember(expected, actual) || record.After == nil || actual.Rect != *record.After {
			outcome.Message = "Fenetre ou position modifiee depuis le geste : restauration ignoree"
			return outcome
		}
	}
	hwnd, err := strconv.ParseUint(record.WindowID, 10, strconv.IntSize)
	if err == nil {
		err = winapi.SetVisibleWindowRect(uintptr(hwnd), record.Rect)
	}
	if err != nil {
		outcome.Status, outcome.Message = "failed", err.Error()
		return outcome
	}
	actual, err = (trackingBackend{}).Read(record.WindowID)
	if err != nil || actual.Rect != record.Rect || actual.ProcessID != record.ProcessID || actual.Started != record.Started {
		outcome.Status, outcome.Message = "uncertain", "Restauration non confirmee"
		return outcome
	}
	outcome.Status, outcome.Message = "success", "Position d'origine restauree et verifiee"
	return outcome
}
