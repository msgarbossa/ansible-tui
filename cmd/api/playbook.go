package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type PlaybookMetrics struct {
	ExitCode int
	Error    error
}

// struct for storing and passing playbook configurations (marshal, unmarshal, methods, function calls)
type PlaybookConfig struct {
	Playbook          string `yaml:"playbook" json:"playbook"`
	VerboseLevel      int    `yaml:"verbose_level" json:"verbose_level"`
	SshPrivateKeyFile string `yaml:"ssh_private_key_file" json:"ssh_private_key_file"`
	RemoteUser        string `yaml:"remote_user" json:"remote_user"`
	InventoryFile     string `yaml:"inventory" json:"inventory"`
	LimitHost         string `yaml:"limit" json:"limit"`
	ExtraVarsFile     string `yaml:"extra_vars_file" json:"extra_vars_file"`
	AnsibleTags       string `yaml:"tags" json:"tags"`
	AnsibleSkipTags   string `yaml:"skip_tags" json:"skip_tags"`
	ExtraArgs         string `yaml:"extra_args" json:"extra_args"`
	WindowsGroup      string `yaml:"windows_group" json:"windows_group"`
	Image             string `yaml:"image" json:"image"`
	VirtualEnvPath    string `yaml:"virtual_env_path" json:"virtual_env_path"`
	Metrics           PlaybookMetrics
}

func handlePlaybookParams(pbConfigFile string) (int, error) {

	// playbook config
	pb := PlaybookConfig{}
	rc := 0

	// read config file defined by flag or environment variable into PlaybookConfig struct
	err := pb.readConf(pbConfigFile)
	if err != nil {
		slog.Error("Exiting due to config file error: %s", err)
		return 1, err
	}

	// read environment variables into PlaybookConfig struct
	err = pb.readEnvs()
	if err != nil {
		slog.Error("Exiting due error reading environment variables: %s", err)
		return 1, err
	}

	// validate inputs in PlaybookConfig struct
	err = pb.validateInputs()
	if err != nil {
		slog.Error("Exiting due validation errors: %s", err)
		return 1, err
	}

	// Using PlaybookConfig struct, determine runtime environment (ansible in path, in Python venv, or container).
	// If container execution, write struct to file, run ansible-shim inside container to read and execute ansible.
	rc, err = pb.runAnsiblePlaybook()
	if err != nil {
		slog.Error("Error running playbook: %s", err)
		return rc, err
	}

	return rc, nil

}

func (c *PlaybookConfig) readConf(pbConfigFile string) error {

	// no config file
	if pbConfigFile == "" {
		slog.Debug("No config file defined")
		return nil
	}

	buf, err := os.ReadFile(pbConfigFile)
	if err != nil {
		slog.Error("Error reading config file %s: %s", pbConfigFile, err)
		return err
	}

	//c := &Config{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		slog.Error("Error unmarshalling config file %s: %s", pbConfigFile, err)
		return err
	}

	return nil
}

// PlaybookConfig method to validate captured inputs from PlaybookConfig struct
func (c *PlaybookConfig) validateInputs() error {

	if c.VerboseLevel < 0 || c.VerboseLevel > 4 {
		slog.Warn("VERBOSE_LEVEL must be between 0 and 4, using default (1).")
		c.VerboseLevel = 1
	}

	switch c.VerboseLevel {
	case 0:
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case 1:
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case 2, 3, 4:
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if c.Playbook == "" {
		return &InputError{
			Err: errors.New("playbook is required"),
		}
	}

	err := sanitizePath(c.Playbook, false)
	absPath, _ := filepath.Abs(c.Playbook)
	slog.Info(fmt.Sprintf("Checking playbook path: %s", absPath))
	if err != nil {
		slog.Error(fmt.Sprintf("Error validating playbook path: %s", absPath))
		return err
	}

	if c.InventoryFile == "" {
		return &InputError{
			Err: errors.New("inventory parameter is required"),
		}
	}

	err = sanitizePath(c.InventoryFile, false)
	absPath, _ = filepath.Abs(c.InventoryFile)
	slog.Info(fmt.Sprintf("Checking inventory path: %s", absPath))
	if err != nil {
		slog.Error(fmt.Sprintf("Error validating inventory path: %s", absPath))
		return err
	}

	if c.ExtraVarsFile != "" {
		err := sanitizePath(c.ExtraVarsFile, false)
		if err != nil {
			absPath, _ = filepath.Abs(c.ExtraVarsFile)
			slog.Error(fmt.Sprintf("Error validating extra-vars: %s", absPath))
			slog.Error("Error validating extra-vars")
			return err
		}
	}

	execTypeCount := 0
	if c.VirtualEnvPath != "" {
		if strings.HasPrefix(c.VirtualEnvPath, "~") {
			home := os.Getenv("HOME")
			c.VirtualEnvPath = strings.Replace(c.VirtualEnvPath, "~", home, 1)
		}
		err := sanitizePath(c.VirtualEnvPath, true)
		if err != nil {
			slog.Error("Error validating path to virtual environment directory")
			return err
		}
		//c.VirtualEnvPath = filepath.Dir(c.VirtualEnvPath)
		ansiblePlaybookPath := filepath.Join(c.VirtualEnvPath, "bin", "ansible-playbook")
		if _, err := os.Stat(ansiblePlaybookPath); os.IsNotExist(err) {
			slog.Error("ansible-playbook is NOT in a Python virtual environment")
			return err
		} else {
			slog.Info("ansible-playbook is in the specified Python virtual environment")
		}
		ansibleInventoryPath := filepath.Join(c.VirtualEnvPath, "bin", "ansible-inventory")
		if _, err := os.Stat(ansibleInventoryPath); os.IsNotExist(err) {
			slog.Error("ansible-inventory is NOT in a Python virtual environment")
			return err
		}
		execTypeCount++
	}

	if c.Image != "" {
		slog.Info(fmt.Sprintf("image is set to %s", c.Image))
		execTypeCount++
	}

	if execTypeCount > 1 {
		return &InputError{
			Err: errors.New("python_path and container_image are mutually exclusive (only specify one)"),
		}
	}

	if c.SshPrivateKeyFile != "" {
		if strings.HasPrefix(c.SshPrivateKeyFile, "~") {
			home := os.Getenv("HOME")
			c.SshPrivateKeyFile = strings.Replace(c.SshPrivateKeyFile, "~", home, 1)
		}
		err := sanitizePath(c.SshPrivateKeyFile, false)
		if err != nil {
			absPath, _ = filepath.Abs(c.SshPrivateKeyFile)
			slog.Error(fmt.Sprintf("Error validating path to SSH private key: %s", absPath))
			return err
		}
	}

	// TODO: To enforce /etc/ansible/ansible.cfg in container image, disallow (and log warning) for any of these:
	// ANSIBLE_CONFIG (environment variable if set)
	// ansible.cfg (in the current directory)
	// ~/.ansible.cfg (in the home directory)

	return nil
}

// PlaybookConfig method to read environment variables into PlaybookConfig struct
func (c *PlaybookConfig) readEnvs() error {

	playbook := os.Getenv("PLAYBOOK")
	if playbook != "" {
		c.Playbook = playbook
	}

	verboseLevel := os.Getenv("VERBOSE_LEVEL")
	if verboseLevel != "" {
		verboseLevelInt, err := strconv.Atoi(verboseLevel)
		if err != nil {
			slog.Warn("Could not convert VERBOSE_LEVEL to integer, using default (1)")
			verboseLevelInt = 1
		}
		c.VerboseLevel = verboseLevelInt
	}

	sshPrivateKeyFile := os.Getenv("SSH_PRIVATE_KEY_FILE")
	if sshPrivateKeyFile != "" {
		c.SshPrivateKeyFile = sshPrivateKeyFile
	}

	remoteUser := os.Getenv("ANSIBLE_REMOTE_USER")
	if remoteUser != "" {
		c.RemoteUser = remoteUser
	}

	inventoryFile := os.Getenv("INVENTORY_FILE")
	inventoryContents := os.Getenv("INVENTORY_CONTENTS")
	inventoryUrl := os.Getenv("INVENTORY_URL")

	invInputCount := 0
	if inventoryFile != "" {
		c.InventoryFile = inventoryFile
		invInputCount++
	}

	if inventoryContents != "" {
		c.InventoryFile = "./hosts-INVENTORY"
		// write out contents to file
		err := writeFileFromString(c.InventoryFile, inventoryContents)
		if err != nil {
			slog.Error("could not write inventory file from inventory contents")
			return err
		}
		invInputCount++
	}

	if invInputCount > 1 {
		return &InputError{
			Err: errors.New("only one inventory environment variable is allowed"),
		}
	}

	if inventoryUrl != "" {
		c.InventoryFile = "./hosts-INVENTORY"
		// TODO: get inventory from url and write to file
		invInputCount++
	}

	if invInputCount > 1 {
		return &InputError{
			Err: errors.New("only one inventory environment variable is allowed"),
		}
	}

	limitHost := os.Getenv("LIMIT_HOST")
	if limitHost != "" {
		c.LimitHost = limitHost
	}

	varInputCount := 0
	extraVarsFile := os.Getenv("EXTRA_VARS_FILE")
	if extraVarsFile != "" {
		c.ExtraVarsFile = extraVarsFile
		varInputCount++
	}

	extraVarsContents := os.Getenv("EXTRA_VARS_CONTENTS")
	if extraVarsContents != "" {
		c.ExtraVarsFile = "PLAYBOOK-extravars"
		// write out contents to file
		err := writeFileFromString(c.ExtraVarsFile, extraVarsContents)
		if err != nil {
			slog.Error("could not write extra-vars file from extra-vars contents")
			return err
		}
		varInputCount++
	}

	if varInputCount > 1 {
		return &InputError{
			Err: errors.New("only one extra-vars environment variable is allowed"),
		}
	}

	ansibleTags := os.Getenv("ANSIBLE_TAGS")
	if ansibleTags != "" {
		c.AnsibleTags = ansibleTags
	}

	ansibleSkipTags := os.Getenv("ANSIBLE_SKIP_TAGS")
	if ansibleSkipTags != "" {
		c.AnsibleSkipTags = ansibleSkipTags
	}

	extraArgs := os.Getenv("EXTRA_ARGS")
	if extraArgs != "" {
		c.ExtraArgs = extraArgs
	}

	windowsGroups := os.Getenv("WINDOWS_GROUP")
	if windowsGroups != "" {
		c.WindowsGroup = windowsGroups
	}

	virtualEnvPath := os.Getenv("VIRTUAL_ENV")
	if virtualEnvPath != "" {
		c.VirtualEnvPath = virtualEnvPath
	}

	containerImage := os.Getenv("CONTAINER_IMAGE")
	if containerImage != "" {
		c.Image = containerImage
	}

	return nil
}

func executePlaybookInContainer(c PlaybookConfig) (int, error) {

	slog.Debug("Starting executePlaybookInContainer()")

	containerCmd := ""

	// Need to determine container runtime
	_, err := exec.LookPath("docker")
	if err == nil {
		slog.Debug("found docker")
		containerCmd = "docker"
	}
	_, err = exec.LookPath("podman")
	if err == nil {
		slog.Debug("found podman")
		containerCmd = "podman"
	}
	if containerCmd == "" {
		return 1, &ExecutionError{
			Err: errors.New("container image was specified, but no container runtime could be found (docker or podman)"),
		}
	}
	slog.Info(fmt.Sprintf("Using %s for container runtime", containerCmd))

	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("could not get current working directory")
		return 1, err
	}
	volMount1 := cwd + ":" + "/app:rw,z"

	var containerArgs []string
	containerArgs = append(containerArgs, "run", "--rm", "-u", "root", "-e", "PB_CONFIG_FILE=/app/container-config.yml", "-v", volMount1)

	if c.SshPrivateKeyFile != "" {
		c.SshPrivateKeyFile, err = filepath.EvalSymlinks(c.SshPrivateKeyFile)
		if err != nil {
			return 1, err
		}
		// set volume mount to normalized location in container and append to command
		volMount2 := c.SshPrivateKeyFile + ":" + "/app/.ssh/ansible-shim:ro,z"
		containerArgs = append(containerArgs, "-v", volMount2)

		// modify SSH private key path to location used inside the container
		c.SshPrivateKeyFile = "/app/.ssh/ansible-shim"
	}

	containerArgs = append(containerArgs, c.Image)

	// unset image in PlaybookConfig before marshal for execution inside container
	c.Image = ""

	// write PlaybookConfig just before execution since values were modified accordingly above
	b, err := yaml.Marshal(&c)
	if err != nil {
		slog.Error("Could not marshal PlaybookConfig to bytes")
		return 1, err
	}

	containerConfigFile := "./container-config.yml"
	err = os.WriteFile(containerConfigFile, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", containerConfigFile))
		return 1, err
	}

	// get timeout value, which if set will be used below to set context.WithTimeout
	ansiblePlaybookTimeout, err := strconv.Atoi(LookupEnvOrString("ANSIBLE_PLAYBOOK_TIMEOUT", "-1"))
	if err != nil {
		slog.Error("Could not convert ANSIBLE_PLAYBOOK_TIMEOUT to integer")
		return 1, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	if ansiblePlaybookTimeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(ansiblePlaybookTimeout)*time.Second)
	}

	defer func() {
		for {
			select {
			case <-ctx.Done():
				// fmt.Println(ctx.Err())
				// slog.Error(fmt.Sprintf("executePlaybookInContainer context: %s", ctx.Err()))
				return
			default:
				// fmt.Println("finished")
				// slog.Info("executePlaybookInContainer context finished")
				cancel()
			}
		}
	}()

	slog.Debug(fmt.Sprintf("%s %s", containerCmd, containerArgs))

	cmd := exec.CommandContext(ctx, containerCmd, containerArgs...)

	slog.Info(cmd.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
		//panic(err)
	}
	scanner := bufio.NewScanner(stdout)
	err = cmd.Start()
	pid := cmd.Process.Pid
	slog.Info(fmt.Sprintf("PID for container run command: %d", pid))
	if err != nil {
		slog.Error("cmd.Start() failed")
		return 1, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		// slog.Info(line)
		fmt.Println(line)
	}
	if scanner.Err() != nil {
		cmd.Process.Kill()
		cmd.Wait()
		// panic(scanner.Err())
		slog.Error("scanner error")
		return 1, scanner.Err()
	}

	rc := 0
	if err := cmd.Wait(); err != nil {
		slog.Info(fmt.Sprintf("Checking status: %s", err))
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			slog.Info(exiterr.String())
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", exiterr))
			// rc = exiterr.ExitCode()
			rc = 1
		}
	}

	slog.Info(fmt.Sprintf("container finished: rc=%d", rc))
	return rc, err

}

// PlaybookConfig method to read PlaybookConfig struct, formulate and run ansible-playbook command
func (c *PlaybookConfig) runAnsiblePlaybook() (int, error) {

	var (
		err error
	)

	slog.Debug("Starting runAnsiblePlaybook()")

	PrintMemUsage()

	rc := 0

	// container image was specified
	if c.Image != "" {
		rc, err = executePlaybookInContainer(*c)
		slog.Info(fmt.Sprintf("Finished executePlaybookInContainer: rc=%d", rc))
		return rc, err
	}

	ansibleCmdPath := "ansible-playbook"

	if c.VirtualEnvPath != "" {
		os.Setenv("PATH", c.VirtualEnvPath+"/bin:"+os.Getenv("PATH"))
		slog.Info("Using virtualenv for " + ansibleCmdPath)
	}

	path, err := exec.LookPath("ansible-playbook")
	if err != nil {
		slog.Warn(fmt.Sprintf("ansible-playbook lookup err: %s", err))
		slog.Info(os.Getenv("PATH"))
		return 1, err
	} else {
		slog.Info(fmt.Sprintf("ansible-playbook lookup path: %s", path))
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

	// TODO: Validate c.InventoryFile
	// err := validateInventory(c.InventoryFile)
	// if err != nil {
	// 	slog.Error("Exiting due to inventory file validation error: %s", err)
	// 	return err
	// }

	ansiblePlaybookArgs = append(ansiblePlaybookArgs, "-i", c.InventoryFile, c.Playbook)

	if c.LimitHost != "" {
		ansiblePlaybookArgs = append(ansiblePlaybookArgs, "--limit", c.LimitHost)
	}

	ansiblePlaybookTimeout, err := strconv.Atoi(LookupEnvOrString("ANSIBLE_PLAYBOOK_TIMEOUT", "-1"))
	if err != nil {
		slog.Error("Could not convert ANSIBLE_PLAYBOOK_TIMEOUT to integer")
		return 1, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	if ansiblePlaybookTimeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(ansiblePlaybookTimeout)*time.Second)
	}

	defer func() {
		for {
			select {
			case <-ctx.Done():
				// fmt.Println(ctx.Err())
				// slog.Error(fmt.Sprintf("runAnsiblePlaybook context: %s", ctx.Err()))
				return
			default:
				//fmt.Println("finished")
				// slog.Info("runAnsiblePlaybook context finished")
				cancel()
			}
		}
	}()

	slog.Debug(fmt.Sprintf("%s %s", ansibleCmdPath, ansiblePlaybookArgs))

	cmd := exec.CommandContext(ctx, ansibleCmdPath, ansiblePlaybookArgs...)

	slog.Info(cmd.String())
	cmd.Stderr = os.Stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
		//panic(err)
	}

	slog.Debug("starting playbook execution")
	err = cmd.Start()
	pid := cmd.Process.Pid
	slog.Info(fmt.Sprintf("PID for ansible-playbook command: %d", pid))
	if err != nil {
		slog.Error("cmd.Start() failed")
		return 1, err
	}

	r := bufio.NewReader(stdout)

	truncating := false
	for {
		line_b, isPrefix, err := r.ReadLine()
		line := string(line_b)
		if isPrefix && truncating {
			continue
		}
		truncating = false
		if isPrefix && !truncating {
			line = line + " [TRUNCATED]"
			truncating = true
		}
		if err != nil {
			slog.Error(fmt.Sprintf("ReadLine error: %s", err))
			break
		}

		fmt.Println(line)
	}

	// scanner := bufio.NewScanner(stdout)
	// buf := make([]byte, 0, 64*1024)
	// scanner.Buffer(buf, 1024*1024)

	// for scanner.Scan() {
	// 	line := scanner.Text()
	// 	// Handle each line of output
	// 	fmt.Println(line)
	// 	// slog.Info(line)
	// }
	// if err := scanner.Err(); err != nil {
	// 	// process the error
	// 	slog.Error(fmt.Sprintf("scanner error: %s", err))
	// 	cmd.Process.Kill()
	// 	cmd.Wait()
	// 	// scanner.Err() == bufio.ErrTooLong
	// 	// panic(scanner.Err())
	// 	return 1, scanner.Err()
	// }

	// if scanner.Err() != nil {
	// 	cmd.Process.Kill()
	// 	cmd.Wait()
	// 	// scanner.Err() == bufio.ErrTooLong
	// 	slog.Error("scanner error")
	// 	// panic(scanner.Err())
	// 	return 1, scanner.Err()
	// }

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", err))
			// rc = exiterr.ExitCode()
			rc = 1
		}
	}

	PrintMemUsage()

	slog.Info(fmt.Sprintf("ansible-playbook finished: rc=%d", rc))
	return rc, err
}

// PrintMemUsage outputs the current, total and OS memory being used. As well as the number
// of garbage collection cycles completed.
func PrintMemUsage() {

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
