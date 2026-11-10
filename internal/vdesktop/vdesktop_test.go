package vdesktop

import (
	"errors"
	"testing"

	"deskctl/internal/winapi"
)

const testDesktopID = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"

var testDesktops = []winapi.VirtualDesktopInfo{{Index: 0, ID: testDesktopID, Name: "Dev"}}

func TestValidateLayoutAndLegacyFormat(t *testing.T) {
	valid := `[{"ProcessId":1234,"TargetDesktopId":"` + testDesktopID + `"}]`
	entries, err := ValidateLayout([]byte(valid))
	if err != nil || len(entries) != 1 || entries[0].TargetDesktopID != testDesktopID {
		t.Fatal("valid GUID layout rejected", err)
	}
	if _, err := ValidateLayout([]byte(`[{"ProcessId":1234,"TargetDesktopName":"Dev_A"}]`)); err == nil {
		t.Fatal("legacy name-based layout must request regeneration")
	}
	for _, invalid := range []string{
		`[{"ProcessId":0,"TargetDesktopId":"` + testDesktopID + `"}]`,
		`[{"ProcessId":1,"TargetDesktopId":"bad-guid"}]`,
		`[{"ProcessId":1,"TargetDesktopId":"` + testDesktopID + `"},{"ProcessId":1,"TargetDesktopId":"` + testDesktopID + `"}]`,
	} {
		if _, err := ValidateLayout([]byte(invalid)); err == nil {
			t.Fatalf("invalid layout accepted: %s", invalid)
		}
	}
}

func TestValidateReferenceRequiresMatchingPIDsAndDesktopIDs(t *testing.T) {
	entries := []LayoutEntry{{ProcessId: 10, TargetDesktopID: testDesktopID}}
	windows := []winapi.WindowInfo{{ProcessId: 10, ProcessName: "Code", DesktopID: testDesktopID, WindowID: 1}}
	if references, err := ValidateReference(entries, windows); err != nil || references[10] != "Code" {
		t.Fatal("valid inventory reference rejected", err)
	}
	if _, err := ValidateReference(entries, nil); err == nil {
		t.Fatal("missing PID reference must be rejected")
	}
	windows = append(windows, winapi.WindowInfo{ProcessId: 10, ProcessName: "Code", DesktopID: testDesktopID, WindowID: 2})
	if _, err := ValidateReference(entries, windows); err != nil {
		t.Fatal("multiple windows for one PID must be accepted", err)
	}
	windows = windows[:1]
	windows[0].DesktopID = ""
	if _, err := ValidateReference(entries, windows); err == nil {
		t.Fatal("inventory without DesktopId must be rejected")
	}
	if _, err := ValidateDesktopTargets(entries, []winapi.VirtualDesktopInfo{{Index: 0, ID: "12345678-1234-1234-1234-1234567890ab", Name: "Other"}}); err == nil {
		t.Fatal("target GUID absent from live desktop list must be rejected")
	}
	if names, err := ValidateDesktopTargets(entries, testDesktops); err != nil || names[testDesktopID] != "Dev" {
		t.Fatal("target name should be resolved from the desktop list", err)
	}
}

func TestApplyLayoutSimulationAndPIDProtection(t *testing.T) {
	entries := []LayoutEntry{
		{ProcessId: 10, TargetDesktopID: testDesktopID},
		{ProcessId: 20, TargetDesktopID: testDesktopID},
		{ProcessId: 30, TargetDesktopID: testDesktopID},
	}
	windows := []winapi.WindowInfo{
		{ProcessId: 10, ProcessName: "Code", WindowID: 100, WindowHandle: 100},
		{ProcessId: 10, ProcessName: "Code", WindowID: 101, WindowHandle: 101},
		{ProcessId: 20, ProcessName: "Other", WindowID: 200, WindowHandle: 200},
	}
	moveCalls := 0
	bilan, err := ApplyLayout(func(uintptr, string) error {
		moveCalls++
		return nil
	}, entries, windows, testDesktops, map[int]string{10: "Code", 20: "OldProcess"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if moveCalls != 0 || bilan.Success != 2 || bilan.Conflicts != 1 || bilan.Ignored != 1 {
		t.Fatalf("simulation or PID protection failed: calls=%d bilan=%+v", moveCalls, bilan)
	}
}

func TestApplyLayoutUsesWindowsMover(t *testing.T) {
	windows := []winapi.WindowInfo{
		{ProcessId: 10, ProcessName: "Code", WindowID: 100, WindowHandle: 100},
		{ProcessId: 10, ProcessName: "Code", WindowID: 101, WindowHandle: 101},
	}
	called := 0
	bilan, err := ApplyLayout(func(hwnd uintptr, desktopID string) error {
		called++
		if hwnd != windows[called-1].WindowHandle || desktopID != testDesktopID {
			t.Fatalf("unexpected move request: hwnd=%d desktop=%s", hwnd, desktopID)
		}
		return nil
	}, []LayoutEntry{{ProcessId: 10, TargetDesktopID: testDesktopID}}, windows, testDesktops, nil, true)
	if err != nil || called != 2 || bilan.Success != 2 {
		t.Fatal("real mode must use the injected Windows API mover", err)
	}
	if _, err := ApplyLayout(nil, []LayoutEntry{{ProcessId: 10, TargetDesktopID: testDesktopID}}, windows, testDesktops, nil, true); err == nil {
		t.Fatal("real mode without a mover must fail")
	}
	failure := errors.New("desktop no longer exists")
	bilan, err = ApplyLayout(func(uintptr, string) error { return failure }, []LayoutEntry{{ProcessId: 10, TargetDesktopID: testDesktopID}}, windows[:1], testDesktops, nil, true)
	if err != nil || bilan.Failures != 1 {
		t.Fatal("invalid or stale desktop GUID must be reported as a move failure", err)
	}
}
