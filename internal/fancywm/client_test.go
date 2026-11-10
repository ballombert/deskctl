package fancywm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type commandCall struct {
	executable string
	args       []string
	output     []byte
	err        error
}

type fakeRunner struct{ call commandCall }

func (runner *fakeRunner) Run(_ context.Context, executable string, args ...string) ([]byte, error) {
	runner.call.executable = executable
	runner.call.args = args
	return runner.call.output, runner.call.err
}

func TestMoveToDisplayUsesDocumentedAction(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{
		LookPath: func(name string) (string, error) {
			if name != "fancywm" {
				t.Fatalf("unexpected executable lookup: %s", name)
			}
			return `C:\Tools\fancywm.exe`, nil
		},
		Runner: runner,
	}
	if err := client.MoveToDisplay(context.Background(), "fancywm", 3); err != nil {
		t.Fatal(err)
	}
	if runner.call.executable != `C:\Tools\fancywm.exe` || !reflect.DeepEqual(runner.call.args, []string{"--action", "MoveToDisplay3"}) {
		t.Fatalf("unexpected FancyWM invocation: %+v", runner.call)
	}
}

func TestMoveToDisplayRejectsInvalidIndexAndUnavailableClient(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{LookPath: func(string) (string, error) { return "fancywm.exe", nil }, Runner: runner}
	for _, index := range []int{0, 10} {
		if err := client.MoveToDisplay(context.Background(), "fancywm", index); err == nil {
			t.Fatalf("expected display %d to be rejected", index)
		}
	}
	if runner.call.executable != "" {
		t.Fatal("invalid display index must not launch FancyWM")
	}

	client.LookPath = func(string) (string, error) { return "", errors.New("not installed") }
	if err := client.MoveToDisplay(context.Background(), "fancywm", 1); err == nil {
		t.Fatal("missing FancyWM must be reported")
	}
}

func TestMoveToDisplayReturnsFancyWMFailure(t *testing.T) {
	client := Client{
		LookPath: func(string) (string, error) { return "fancywm.exe", nil },
		Runner:   &fakeRunner{call: commandCall{output: []byte("window refused"), err: errors.New("exit status 1")}},
	}
	if err := client.MoveToDisplay(context.Background(), "fancywm", 2); err == nil || !strings.Contains(err.Error(), "window refused") {
		t.Fatalf("expected FancyWM failure output, got %v", err)
	}
}

func TestActionWhitelist(t *testing.T) {
	runner := &fakeRunner{}
	client := Client{LookPath: func(string) (string, error) { return "fancywm.exe", nil }, Runner: runner}
	for _, action := range []string{"Shutdown", "MoveToDisplay0", "CreateVerticalPanel; calc", ""} {
		if err := client.Action(context.Background(), "fancywm", action); err == nil {
			t.Fatalf("action %q must be refused", action)
		}
	}
	if runner.call.executable != "" {
		t.Fatal("refused actions must not launch FancyWM")
	}
	if err := client.Action(context.Background(), "fancywm", "CreateVerticalPanel"); err != nil || runner.call.args[1] != "CreateVerticalPanel" {
		t.Fatal("documented action refused", err)
	}
}
