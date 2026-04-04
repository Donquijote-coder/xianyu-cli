//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

func setSetsid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000010, // CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
	}
}

