package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type PlaybookMetrics struct {
	ExitCode       int
	Error          error
	InventoryCount int
}

type PlaybookEnvironmentVariables struct {
	Pass []string          `json:"pass"`
	Set  map[string]string `json:"set"`
}

// struct for storing and passing playbook configurations (marshal, unmarshal, methods, function calls)
type PlaybookConfig struct {
	Playbook             string                       `yaml:"playbook" json:"playbook"`
	VerboseLevel         int                          `yaml:"verbose-level" json:"verbose-level"`
	SshPrivateKeyFile    string                       `yaml:"ssh-private-key-file" json:"ssh-private-key-file"`
	RemoteUser           string                       `yaml:"remote-user" json:"remote-user"`
	InventoryFile        string                       `yaml:"inventory" json:"inventory"`
	LimitHost            string                       `yaml:"limit" json:"limit"`
	ExtraVarsFile        string                       `yaml:"extra-vars-file" json:"extra-vars-file"`
	AnsibleTags          string                       `yaml:"tags" json:"tags"`
	AnsibleSkipTags      string                       `yaml:"skip-tags" json:"skip-tags"`
	ExtraArgs            string                       `yaml:"extra-args" json:"extra-args"`
	WindowsGroup         string                       `yaml:"windows-group" json:"windows-group"`
	ExecutionType        string                       `yaml:"execution-type" json:"execution-type"`
	Image                string                       `yaml:"image" json:"image"`
	VirtualEnvPath       string                       `yaml:"virtual-env-path" json:"virtual-env-path"`
	EnvironmentVariables PlaybookEnvironmentVariables `yaml:"environment-variables"`
	Metrics              PlaybookMetrics
}

var (
	tempDirPath = "./.ansible-shim"
)

func handlePlaybookParams(pbConfigFile string) (int, error) {

	// playbook config
	pb := PlaybookConfig{}
	rc := 0

	// read config file defined by flag or environment variable into PlaybookConfig struct
	err := pb.readConf(pbConfigFile)
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to config file error: %s", err))
		return 1, err
	}

	// read environment variables into PlaybookConfig struct
	err = pb.readEnvs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due error reading environment variables: %s", err))
		return 1, err
	}

	// validate inputs in PlaybookConfig struct
	err = pb.validateInputs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due validation errors: %s", err))
		return 1, err
	}

	// process values in PlaybookConfig struct
	err = pb.processInputs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to errors processing inputs: %s", err))
		return 1, err
	}

	// Using PlaybookConfig struct, determine runtime environment (ansible in path, in Python venv, or container).
	// If container execution, write struct to file, run ansible-shim inside container to read and execute ansible.
	rc, err = pb.runAnsiblePlaybook()
	if err != nil {
		slog.Error(fmt.Sprintf("Error running playbook: %s", err))
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
		slog.Error(fmt.Sprintf("Error reading config file %s: %s", pbConfigFile, err))
		return err
	}

	//c := &Config{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		slog.Error(fmt.Sprintf("Error unmarshalling config file %s: %s", pbConfigFile, err))
		return err
	}

	return nil
}

// PlaybookConfig method to validate captured inputs from PlaybookConfig struct
func (c *PlaybookConfig) validateInputs() error {

	if c.VerboseLevel < 0 || c.VerboseLevel > 7 {
		slog.Warn("VERBOSE_LEVEL must be between 0 and 7, using default (1).")
		c.VerboseLevel = 1
	}

	switch v := c.VerboseLevel; {
	case v == 0:
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case v == 1:
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case v > 1:
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	var (
		err     error
		absPath string
	)

	if c.Playbook == "" {
		return &InputError{
			Err: errors.New("playbook is required"),
		}
	}

	slog.Info(fmt.Sprintf("Checking playbook path: %s", c.Playbook))
	err = sanitizePath(c.Playbook)
	if err != nil {
		slog.Error(fmt.Sprintf("sanitizing playbook path: %s", c.Playbook))
		return err
	}
	if ok := checkRelativePath(c.Playbook); !ok {
		return &InputError{
			Err: errors.New("playbook must have relative path to current directory"),
		}
	}
	if ok, err := pathExists(c.Playbook, false); !ok {
		slog.Error(fmt.Sprintf("Path for playbook does not exist: %s", c.Playbook))
		return err
	}

	if c.InventoryFile == "" {
		return &InputError{
			Err: errors.New("inventory parameter is required"),
		}
	}

	slog.Info(fmt.Sprintf("Checking inventory file path: %s", c.InventoryFile))
	err = sanitizePath(c.InventoryFile)
	if err != nil {
		slog.Error(fmt.Sprintf("Error sanitizing inventory file path: %s", c.InventoryFile))
		return err
	}
	if ok := checkRelativePath(c.InventoryFile); !ok {
		return &InputError{
			Err: errors.New("inventory-file must have relative path to current directory"),
		}
	}
	if ok, err := pathExists(c.InventoryFile, false); !ok {
		slog.Error(fmt.Sprintf("Inventory path file does not exist: %s", c.InventoryFile))
		return err
	}

	if c.ExtraVarsFile != "" {
		slog.Info(fmt.Sprintf("Checking extra-vars file path: %s", c.ExtraVarsFile))
		err := sanitizePath(c.ExtraVarsFile)
		if err != nil {
			slog.Error(fmt.Sprintf("sanitizing extra-vars path: %s", c.ExtraVarsFile))
			return err
		}
		if ok := checkRelativePath(c.ExtraVarsFile); !ok {
			return &InputError{
				Err: errors.New("extra-vars file must have relative path to current directory"),
			}
		}
		if ok, err := pathExists(c.ExtraVarsFile, false); !ok {
			slog.Error(fmt.Sprintf("Path for extra-vars file does not exist: %s", c.ExtraVarsFile))
			return err
		}
	}

	execTypeCount := 0
	if c.VirtualEnvPath != "" {
		slog.Info(fmt.Sprintf("Python virtual environment path: %s", c.VirtualEnvPath))
		execTypeCount++
	}

	if c.Image != "" {
		slog.Info(fmt.Sprintf("image is set to %s", c.Image))
		execTypeCount++
	}

	if execTypeCount > 1 {
		switch e := c.ExecutionType; e {
		case "container":
			slog.Warn("Execution type is set to container, unsetting virtual environment")
			c.VirtualEnvPath = ""
		case "venv":
			slog.Warn("Execution type is set to venv, unsetting image")
			c.Image = ""
		default:
			return &InputError{
				Err: errors.New("python_path and container_image are mutually exclusive (only specify one or set execution-type to container or venv)"),
			}
		}
	}

	if c.VirtualEnvPath != "" {
		slog.Info(fmt.Sprintf("Checking Python virtual environment path: %s", c.VirtualEnvPath))
		if strings.HasPrefix(c.VirtualEnvPath, "~") {
			home := os.Getenv("HOME")
			slog.Info(fmt.Sprintf("HOME=%s", home))
			c.VirtualEnvPath = strings.Replace(c.VirtualEnvPath, "~", home, 1)
		}
		err := sanitizePath(c.VirtualEnvPath)
		if err != nil {
			slog.Error(fmt.Sprintf("santizing path to virtual environment directory: %s", c.VirtualEnvPath))
			return err
		}
		absPath, _ = filepath.Abs(c.VirtualEnvPath)
		if ok, err := pathExists(absPath, true); !ok {
			slog.Error(fmt.Sprintf("Python virtual environment path does not exist: %s", absPath))
			return err
		}

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

	if c.SshPrivateKeyFile != "" {
		slog.Info(fmt.Sprintf("Checking SSH private key path: %s", c.SshPrivateKeyFile))
		if strings.HasPrefix(c.SshPrivateKeyFile, "~") {
			home := os.Getenv("HOME")
			c.SshPrivateKeyFile = strings.Replace(c.SshPrivateKeyFile, "~", home, 1)
		}
		err := sanitizePath(c.SshPrivateKeyFile)
		if err != nil {
			absPath, _ = filepath.Abs(c.SshPrivateKeyFile)
			slog.Error(fmt.Sprintf("Error sanitizing path to SSH private key: %s", absPath))
			return err
		}
		absPath, _ = filepath.Abs(c.SshPrivateKeyFile)
		if ok, err := pathExists(absPath, false); !ok {
			slog.Error(fmt.Sprintf("SSH private key file path does not exist: %s", absPath))
			return err
		}
		// when container execution, the absoluate path is used to mount to a specific container path
		c.SshPrivateKeyFile = absPath
	}

	// TODO: To enforce /etc/ansible/ansible.cfg in container image, disallow (and log warning) for any of these:
	// ANSIBLE_CONFIG (environment variable if set)
	// ansible.cfg (in the current directory)
	// ~/.ansible.cfg (in the home directory)

	return nil
}

func (c *PlaybookConfig) processInputs() error {

	// This function manipulates environment variables.
	// It should be run after all inputs (envs and config files) are evaluated
	// and the final PlaybookConfig struct is set.
	// It should be run before any os/exec commands are run for containers
	// or running ansible.

	envPass := make(map[string]string)
	envKeyIdx := make(map[string]bool)

	// These env keys are required for a default shell and should not be unset
	autoPassEnvs := make(map[string]bool)
	autoPassEnvs["PATH"] = true
	autoPassEnvs["HOME"] = true

	// Pass through environment variables listed under environment-variables.pass
	// Capture values to be able to pass into container.
	for _, k := range c.EnvironmentVariables.Pass {
		if v := os.Getenv(k); v != "" {
			slog.Debug(fmt.Sprintf("Caching pass through env %s", k))
			envPass[k] = v // save to map before setting later
		} else {
			slog.Warn(fmt.Sprintf("Pass through env not found: %s", k))
		}
	}

	// set anything in environment-variables.set
	for k, v := range c.EnvironmentVariables.Set {
		_, ok := envPass[k]
		if ok {
			slog.Warn(fmt.Sprintf("Set env also exists in pass, overriding with set value: %s", k))
			delete(envPass, k)
		}
		slog.Debug(fmt.Sprintf("Set env %s = %s", k, v))
		os.Setenv(k, v)
		envKeyIdx[k] = true
	}

	// set any pass through variables that were not overidden by a set variable
	for k, v := range envPass {
		slog.Debug(fmt.Sprintf("Passing through env %s", k))
		c.EnvironmentVariables.Set[k] = v
		envKeyIdx[k] = true
	}

	// clear list of "pass through" variables since they were converted to "set"
	c.EnvironmentVariables.Pass = c.EnvironmentVariables.Pass[:0]

	// delete all other environment variables
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if _, ok1 := envKeyIdx[pair[0]]; !ok1 {
			if _, ok2 := autoPassEnvs[pair[0]]; !ok2 {
				slog.Debug(fmt.Sprintf("Unsetting unspecified env: %s", pair[0]))
				os.Unsetenv(pair[0])
			}
		}
	}

	return nil
}

// PlaybookConfig method to read environment variables into PlaybookConfig struct
func (c *PlaybookConfig) readEnvs() error {

	var (
		err error
	)

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
		// TODO: should probably evaluate this right away in a separate function
	}

	sshPrivateKeyFile := os.Getenv("SSH_PRIVATE_KEY_FILE")
	if sshPrivateKeyFile != "" {
		c.SshPrivateKeyFile = sshPrivateKeyFile
	}

	remoteUser := os.Getenv("ANSIBLE_REMOTE_USER")
	if remoteUser != "" {
		c.RemoteUser = remoteUser
	}

	// create temp-dir if it does not exist
	cwd, _ := os.Getwd()
	p := filepath.Join(cwd, tempDirPath)
	ok, _ := pathExists(p, true)
	if !ok {
		slog.Debug(fmt.Sprintf("Creating temp-dir: %s", p))
		err = ensureDir(p)
		if err != nil {
			slog.Error(fmt.Sprintf("Error creating temp-dir path: %s", p))
			return err
		}
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

func (c *PlaybookConfig) validateAnsibleInventory() error {

	// One challenge here is that ansible-inventory returns a zero return code even when the
	// inventory has errors preventing it from parsing.  Therefore, stderr needs to be parsed
	// for a warning message.

	ansibleInvCmdPath := "ansible-inventory"
	rc := 0

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
		regExpAmp        = regexp.MustCompile(`@`)
		regExpItem       = regexp.MustCompile(`^.*|--(.*)`)
		regExpParseError = regexp.MustCompile(`: Unable to parse`)
		ansibleHosts     = make(map[string]bool)
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
			m := regExpAmp.FindString(strline)
			if m != "" {
				continue
			}

			m = regExpItem.FindString(strline)
			if m != "" {
				ansibleHosts[m] = true
			}
			// slog.Debug(strline)
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
		volMount2 := c.SshPrivateKeyFile + ":" + "/app/.ssh/ansible-shim:ro" // using mount option -z/-Z causes lsetxattr error
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

	// TODO: Should run a separate image pull here so container execution time is more predictable relative to supplied timout.
	// With separate pull can just add a few seconds.

	// get timeout value, which if set will be used below to set context.WithTimeout
	// ansiblePlaybookTimeout, err := strconv.Atoi(LookupEnvOrString("ANSIBLE_PLAYBOOK_TIMEOUT", "-1"))
	// if err != nil {
	// 	slog.Error("Could not convert ANSIBLE_PLAYBOOK_TIMEOUT to integer")
	// 	return 1, err
	// }
	// Set container execution timeout to unlimited until above TODO for image pull is working
	ansiblePlaybookTimeout := -1

	return runBufferedCommandWithoutCapture(containerCmd, containerArgs, ansiblePlaybookTimeout)

}

// PlaybookConfig method to read PlaybookConfig struct, formulate and run ansible-playbook command
func (c *PlaybookConfig) runAnsiblePlaybook() (int, error) {

	var (
		err error
	)

	slog.Debug("Starting runAnsiblePlaybook()")

	// PrintMemUsage()

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

	ansiblePlaybookTimeout, err := strconv.Atoi(LookupEnvOrString("ANSIBLE_PLAYBOOK_TIMEOUT", "-1"))
	if err != nil {
		slog.Error("Could not convert ANSIBLE_PLAYBOOK_TIMEOUT to integer")
		return 1, err
	}

	return runBufferedCommandWithoutCapture(ansibleCmdPath, ansiblePlaybookArgs, ansiblePlaybookTimeout)

}
