package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
)

var (
	regExpInvGroup = regexp.MustCompile(`\@([\w\._-]+)\:$`) // are there char restrictions on group names?
	regExpInvHost  = regexp.MustCompile(`\|--([\w\._-]+)$`) // character limits based on DNS names (RFC 1035)
)

func (c *PlaybookConfig) validateAnsibleInventory() error {

	slog.Debug("Starting validateAnsibleInventory()")

	var err error

	// One challenge here is that ansible-inventory returns a zero return code even when the
	// inventory has errors preventing it from parsing.  Therefore, stderr needs to be parsed
	// for a warning message.

	ansibleInvCmdPath := "ansible-inventory"
	rc := 0

	// TODO: Add support for container image so inventory validation can be independent of playbook execution for TUI
	// container image was specified
	// if c.Image != "" {
	// 	rc, err = executePlaybookInContainer(*c)
	// 	slog.Info(fmt.Sprintf("Finished executePlaybookInContainer: rc=%d", rc))
	// 	return err
	// }

	if c.VirtualEnvPath != "" {
		os.Setenv("PATH", c.VirtualEnvPath+"/bin:"+os.Getenv("PATH"))
		slog.Debug("Using virtualenv for " + ansibleInvCmdPath)
	}

	path, err := exec.LookPath(ansibleInvCmdPath)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s lookup err: %s", ansibleInvCmdPath, err))
		slog.Info(os.Getenv("PATH"))
		return err
	} else {
		slog.Info(fmt.Sprintf("%s lookup path: %s", ansibleInvCmdPath, path))
	}

	var ansibleInventoryArgs []string

	ansibleInventoryArgs = append(ansibleInventoryArgs, "-i", c.InventoryFile, "--graph")

	cmd := exec.Command(ansibleInvCmdPath, ansibleInventoryArgs...)

	slog.Info(cmd.String())

	stderr, err := cmd.StderrPipe()
	if err != nil {
		slog.Error(fmt.Sprintf("Could not get stderr pipe: %v", err))
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error(fmt.Sprintf("Could not get stdout pipe: %v", err))
		return err
	}

	if err := cmd.Start(); err != nil {
		slog.Error(fmt.Sprintf("cmd.Start: %s", err))
		return err
	}

	// To get the inventory count, filter out groups ("@") and look for
	// |--<ansible_host>
	// Store ansible_host in a map to deduplicate, then count map keys.

	var (
		regExpParseError = regexp.MustCompile(`: Unable to parse`)
		ansibleHosts     = make(map[string]bool) // used to deduplicate host to be counted
	)

	defer cmd.Wait()

	go func() {

		reader := bufio.NewReader(stderr)
		for {
			strline, err := readLine(reader)
			if err != nil && err != io.EOF {
				log.Fatal(err)
			}

			// Look for ": Unable to parse"
			m := regExpParseError.FindString(strline)
			if m != "" {
				slog.Error("Parse error detected, setting return code = 1.  Set")
				if c.VerboseLevel < 3 {
					slog.Info("Set verbose-level >= 3 to see ansible-inventory error message.")
				}
				rc = 1
			}

			slog.Debug(strline)
			// fmt.Println(strline)

			if err == io.EOF {
				slog.Debug("EOF.  Done reading buffered stderr for ansible-inventory")
				break
			}
		}
	}()

	go func() {

		reader := bufio.NewReader(stdout)
		for {
			strline, err := readLine(reader)
			if err != nil && err != io.EOF {
				log.Fatal(err)
			}

			// skip lines with "@"
			m := regExpInvGroup.FindString(strline)
			if m != "" {
				continue
			}

			m = regExpInvHost.FindString(strline)
			if m != "" {
				ansibleHosts[m] = true
			}
			slog.Debug(strline)
			// fmt.Println(strline)

			if err == io.EOF {
				slog.Debug("EOF.  Done reading buffered stdout for ansible-inventory")
				break
			}
		}
		c.Metrics.InventoryCount = len(ansibleHosts)
		slog.Info(fmt.Sprintf("inventory count: %d", c.Metrics.InventoryCount))
	}()

	pid := cmd.Process.Pid
	slog.Info(fmt.Sprintf("PID for %s command: %d", ansibleInvCmdPath, pid))

	// err = cmd.Wait()
	// rc = cmd.ProcessState.ExitCode()

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", err))
			// rc = exiterr.ExitCode()
			rc = 1
		}
		slog.Error(fmt.Sprintf("cmd.Wait rc: %d", rc))
	}

	slog.Info(fmt.Sprintf("%s finished: rc=%d", ansibleInvCmdPath, rc))

	if rc != 0 {
		return &InputError{
			Err: errors.New("inventory is not valid"),
		}
	}
	return nil

}

func (c *PlaybookConfig) GetAnsibleInventory(invFilePath string) (*[]string, error) {

	slog.Debug("Starting GetAnsibleInventory()")

	var outputLines *[]string
	var err error
	var rc int

	ansibleInvCmdPath := "ansible-inventory"

	var ansibleInvArgs []string
	ansibleInvArgs = append(ansibleInvArgs, "-i", invFilePath, "--graph")

	// container image was specified
	if c.Image != "" {
		rc, outputLines, err = executeCommandInContainer(*c, ansibleInvCmdPath, ansibleInvArgs, true, "")
		slog.Info(fmt.Sprintf("Finished running ansible-inventory in container: rc=%d", rc))
		return outputLines, err
	}

	if c.VirtualEnvPath != "" {
		os.Setenv("PATH", c.VirtualEnvPath+"/bin:"+os.Getenv("PATH"))
		slog.Info("Using virtualenv for " + ansibleInvCmdPath)
	}

	path, err := exec.LookPath(ansibleInvCmdPath)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s lookup err: %s", ansibleInvCmdPath, err))
		slog.Info(os.Getenv("PATH"))
		return nil, err
	} else {
		slog.Info(fmt.Sprintf("%s lookup path: %s", ansibleInvCmdPath, path))
	}

	rc, outputLines, err = RunBufferedCommand(ansibleInvCmdPath, ansibleInvArgs, 30, true, "")
	slog.Info(fmt.Sprintf("Finished running ansible-inventory: rc=%d", rc))
	return outputLines, err

}

func EvaluateInventoryGraphEntry(line string) (string, string, error) {

	m := regExpInvHost.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1], "host", nil
	}

	m = regExpInvGroup.FindStringSubmatch(line)
	// if m != nil {
	if len(m) > 1 {
		return m[1], "group", nil
	}

	slog.Error(fmt.Sprintf("Could not parse ansible-inventory graph entry: %s", line))

	return "", "", &InputError{
		Err: errors.New("could not parse ansible-inventory graph entry"),
	}
}
