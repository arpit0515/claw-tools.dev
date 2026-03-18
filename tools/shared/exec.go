package shared

import "os/exec"

// execCommand is a thin wrapper so oauth.go doesn't need to import os/exec directly
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
