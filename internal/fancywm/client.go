package fancywm

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Client struct {
	LookPath func(string) (string, error)
	Runner   CommandRunner
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, args...).CombinedOutput()
}

func NewClient() Client {
	return Client{LookPath: exec.LookPath, Runner: OSRunner{}}
}

// Documented FancyWM scripting actions (https://github.com/FancyWM/fancywm/wiki/Scripting).
var allowedAction = regexp.MustCompile(`^(CreateHorizontalPanel|CreateVerticalPanel|CreateStackPanel|PullWindowUp|ToggleFloatingMode|RefreshWorkspace|` +
	`(Move|Swap)(Left|Right|Up|Down)|MoveFocus(Left|Right|Up|Down)|(Increase|Decrease)(Width|Height)|MoveToDisplay[1-9])$`)

// Action runs one whitelisted action on the focused window.
func (client Client) Action(ctx context.Context, executable, action string) error {
	if !allowedAction.MatchString(action) {
		return fmt.Errorf("action FancyWM non autorisee : %q", action)
	}
	if executable == "" {
		return fmt.Errorf("chemin de FancyWM absent dans deskctl.yaml")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookup := client.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	path, err := lookup(executable)
	if err != nil {
		return fmt.Errorf("FancyWM introuvable (%s) : %w", executable, err)
	}
	runner := client.Runner
	if runner == nil {
		runner = OSRunner{}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := runner.Run(ctx, path, "--action", action)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("action FancyWM %s échouée : %w (%s)", action, err, detail)
		}
		return fmt.Errorf("action FancyWM %s échouée : %w", action, err)
	}
	return nil
}

func (client Client) MoveToDisplay(ctx context.Context, executable string, index int) error {
	if index < 1 || index > 9 {
		return fmt.Errorf("indice FancyWM invalide : %d", index)
	}
	return client.Action(ctx, executable, fmt.Sprintf("MoveToDisplay%d", index))
}
