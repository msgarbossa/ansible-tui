package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

func readLine(reader *bufio.Reader) (strLine string, err error) {
	buffer := new(bytes.Buffer)
	for {
		var line []byte
		var isPrefix bool
		// Start reading line
		line, isPrefix, err = reader.ReadLine()

		// slog.Debug(fmt.Sprintf("Read Len: %d, isPrefix: %t, Error: %v\n", len(line), isPrefix, err))

		if err != nil && err != io.EOF {
			return "", err
		}

		buffer.Write(line)

		if !isPrefix {
			// EOL found
			break
		}
	}

	// EOL, return string
	return buffer.String(), err
}

// PlaybookConfig method to read PlaybookConfig struct, formulate and run ansible-playbook command
func (c *PlaybookConfig) RunAnsiblePlaybook() (int, error) {

	var (
		err error
	)

	slog.Debug("Starting RunAnsiblePlaybook()")

	// PrintMemUsage()

	rc := 0

	// container image was specified
	if c.Image != "" {
		rc, _, err = executeCommandInContainer(*c, "/bin/ansible-tui", []string{}, false, "")
		slog.Info(fmt.Sprintf("Finished executePlaybookInContainer: rc=%d", rc))
		return rc, err
	}

	ansibleCmdPath := "ansible-playbook"

	if c.VirtualEnvPath != "" {
		os.Setenv("PATH", c.VirtualEnvPath+"/bin:"+os.Getenv("PATH"))
		slog.Info("Using virtualenv for " + ansibleCmdPath)
	}

	path, err := exec.LookPath(ansibleCmdPath)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s lookup err: %s", ansibleCmdPath, err))
		slog.Info(os.Getenv("PATH"))
		return 1, err
	} else {
		slog.Info(fmt.Sprintf("%s lookup path: %s", ansibleCmdPath, path))
	}

	var ansiblePlaybookArgs []string

	// handle verbose level
	verboseLevel := ""
	if c.VerboseLevel > 0 {
		verboseLevel = "-v"
		for i := 1; i < c.VerboseLevel; i++ {
			verboseLevel += "v"
		}
	}
	if verboseLevel != "" {
		ansiblePlaybookArgs = append(ansiblePlaybookArgs, verboseLevel)
	}

	// set SSH key or else ssh client defaults will be used if SSH is called
	if c.SshPrivateKeyFile != "" {
		os.Setenv("ANSIBLE_PRIVATE_KEY_FILE", c.SshPrivateKeyFile)
	}

	if c.RemoteUser != "" {
		os.Setenv("ANSIBLE_REMOTE_USER", c.RemoteUser)
	}

	// otherwise the color get's lost in Go's tty/command
	os.Setenv("ANSIBLE_FORCE_COLOR", "True")

	err = c.validateAnsibleInventory()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to inventory file validation error: %s", err))
		return 1, err
	}

	// run ansible version

	// if requirements.yml exists, run ansible-galaxy install
	// slog.Debug("setup Ansible roles with ansible-galaxy")
	// ansible-galaxy install -r ./roles/requirements.yml
	// ansible-galaxy install -r ./playbooks/roles/requirements.yml

	ansiblePlaybookArgs = append(ansiblePlaybookArgs, "-i", c.InventoryFile, c.Playbook)

	if c.LimitHost != "" {
		ansiblePlaybookArgs = append(ansiblePlaybookArgs, "--limit", c.LimitHost)
	}

	if c.ExtraArgs != "" {
		slog.Info("Adding extra-args to ansible-playbook command")
		// split string on spaces (also removing \t and \n), append to ansiblePlaybookArgs
		s := strings.Fields(c.ExtraArgs)
		ansiblePlaybookArgs = append(ansiblePlaybookArgs, s...)
	}

	// return RunBufferedCommandWithoutCapture(ansibleCmdPath, ansiblePlaybookArgs, c.PlaybookTimeout)
	rc, _, err = RunBufferedCommand(ansibleCmdPath, ansiblePlaybookArgs, c.PlaybookTimeout, false, "")
	return rc, err

}
