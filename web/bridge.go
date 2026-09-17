package main

import (
	"os"
	"os/exec"
)

func bridgeArgv(region, serial string, extra ...string) []string {
	args := []string{"-region", region, "-deviceSerial", serial}
	if bridgeLogEnabled() {
		args = append(args, "-logFile")
	}
	return append(args, extra...)
}

func applyBridgeEnv(cmd *exec.Cmd, email, password string) {
	cmd.Dir = *workDir
	cmd.Env = append(os.Environ(), "EZVIZ_EMAIL="+email, "EZVIZ_PASSWORD="+password)
}
