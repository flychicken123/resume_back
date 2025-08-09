package handlers

import (
	"os/exec"
)

func runCombined(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = "."
	return cmd.CombinedOutput()
}


