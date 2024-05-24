package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// formulate and run ansible-lint command
func (c *PlaybookConfig) RunAnsibleLint(target string) (int, error) {

	var (
		err error
	)

	slog.Debug("Starting RunAnsibleLint()")

	// PrintMemUsage()

	rc := 0

	ansibleLintFilePath := ""

	if ok, _ := pathExists(ansibleTuiGlobalConfigPath, false); ok {
		d, err := ReadGlobalConfig()
		if err == nil {
			if d.Force.AnsibleLintFilePath != "" {
				ansibleLintFilePath = d.Force.AnsibleLintFilePath
			}
		}
	}

	// TODO: If ansibleLintFilePath is still "" AND AnsibleLintFileUrl is defined,
	// download the file from AnsibleLintFileUrl.

	if ansibleLintFilePath != "" {
		if ok, _ := pathExists(ansibleLintFilePath, false); ok {
			copyFileContents(ansibleLintFilePath, ".ansible-lint")
		}
	}

	// container image was specified
	if c.Image != "" {
		args := []string{"-lp"}
		if target == "." {
			args = []string{"-la"}
		}
		rc, _, err = executeCommandInContainer(*c, "/bin/ansible-tui", args, false, "")
		slog.Info(fmt.Sprintf("Finished executePlaybookInContainer: rc=%d", rc))
		return rc, err
	}

	ansibleLintCmdPath := "ansible-lint"

	if c.VirtualEnvPath != "" {
		os.Setenv("PATH", c.VirtualEnvPath+"/bin:"+os.Getenv("PATH"))
		slog.Info("Using virtualenv for " + ansibleLintCmdPath)
	}

	path, err := exec.LookPath(ansibleLintCmdPath)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s lookup err: %s", ansibleLintCmdPath, err))
		slog.Info(os.Getenv("PATH"))
		return 1, err
	} else {
		slog.Info(fmt.Sprintf("%s lookup path: %s", ansibleLintCmdPath, path))
	}

	ansibleLintArgs := []string{"-v", "-p"}

	if ok, _ := pathExists(".ansible-lint", false); ok {
		ansibleLintArgs = append(ansibleLintArgs, "-c", ".ansible-lint")
	}

	// otherwise the color get's lost in Go's tty/command
	os.Setenv("ANSIBLE_FORCE_COLOR", "True")

	if target != "." {
		ansibleLintArgs = append(ansibleLintArgs, c.Playbook)
	}

	// join ansibleLintCmdPath and ansibleLintArgs and print command
	ansibleLintCmd := ansibleLintCmdPath + " " + strings.Join(ansibleLintArgs, " ")
	slog.Info(fmt.Sprintf("Running: %s", ansibleLintCmd))

	// return RunBufferedCommandWithoutCapture(ansibleCmdPath, ansiblePlaybookArgs, c.PlaybookTimeout)
	rc, _, err = RunBufferedCommand(ansibleLintCmdPath, ansibleLintArgs, c.PlaybookTimeout, false, "")
	return rc, err

}
