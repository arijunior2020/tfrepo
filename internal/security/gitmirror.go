package security

import (
	"context"
	"fmt"
	"os/exec"
)

// Clone performs a full mirror clone of authenticatedURL into dir. dir must
// not already exist — git creates it.
func Clone(ctx context.Context, authenticatedURL, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", "--", authenticatedURL, dir)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --mirror: %w: %s", err, Redact(string(output), authenticatedURL))
	}

	return nil
}

// Push mirrors every ref from the repository in dir to authenticatedURL.
func Push(ctx context.Context, dir, authenticatedURL string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "push", "--mirror", "--", authenticatedURL)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push --mirror: %w: %s", err, Redact(string(output), authenticatedURL))
	}

	return nil
}
