package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// global variables
var (
	BuildVersion   string = ""
	BuildDate      string = ""
	regExpPathName        = regexp.MustCompile(`^[a-zA-Z0-9./\-_]+$`)
	regExpDblDots         = regexp.MustCompile(`\.\.`)
)

// struct for storing and passing playbook configurations (marshal, unmarshal, methods, function calls)
type PlaybookConfig struct {
	Playbook          string `yaml:"playbook"`
	VerboseLevel      int    `yaml:"verbose_level"`
	SshPrivateKeyFile string `yaml:"ssh_private_key_file"`
	RemoteUser        string `yaml:"remote_user"`
	InventoryFile     string `yaml:"inventory"`
	LimitHost         string `yaml:"limit"`
	ExtraVarsFile     string `yaml:"extra_vars_file"`
	AnsibleTags       string `yaml:"tags"`
	AnsibleSkipTags   string `yaml:"skip_tags"`
	ExtraArgs         string `yaml:"extra_args"`
	WindowsGroup      string `yaml:"windows_group"`
	Image             string `yaml:"image"`
	VirtualEnvPath    string `yaml:"virtual_env_path"`
}

type InputError struct {
	Err error
}

type ExecutionError struct {
	Err error
}

func (m *InputError) Error() string {
	return m.Err.Error()
}

func (m *ExecutionError) Error() string {
	return m.Err.Error()
}

// Check path is valid, no special characters, and no "..".
func sanitizePath(path string, dir bool) error {

	if !regExpPathName.MatchString(path) {
		return &InputError{
			Err: errors.New("invalid characters in path"),
		}
	}

	m := regExpDblDots.FindString(path)
	if m != "" {
		return &InputError{
			Err: errors.New("relative paths not allowed"),
		}
	}

	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	slog.Debug(fmt.Sprintf("resolved path: %s", resolvedPath))

	stat, err := os.Stat(resolvedPath)
	if os.IsNotExist(err) {
		return &InputError{
			Err: errors.New("file does not exist"),
		}
	}
	if dir {
		if !stat.IsDir() {
			return &InputError{
				Err: errors.New("path must be a directory"),
			}
		}
	} else {
		if stat.IsDir() {
			return &InputError{
				Err: errors.New("path must be a file, not a directory"),
			}
		}
	}

	return nil
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

func writeFileFromString(outputFilePath string, contents string) error {
	b := []byte(contents) // convert input string (contents) to byte value
	err := os.WriteFile(outputFilePath, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", outputFilePath))
		return err
	}

	return nil
}

func main() {

	// default log level to LevelWarn (prints WARN and ERROR)
	slog.SetLogLoggerLevel(slog.LevelWarn)

	// To allow for testing, os.Exit() should only be done from this main() function.
	// All other functions should return errors (nil when no errors).

	// read and parse CLI options
	pbConfigFile := ""
	displayVersion := flag.Bool("version", false, "Display version and exit")
	flag.StringVar(&pbConfigFile, "c", LookupEnvOrString("PB_CONFIG_FILE", ""), "Playbook config file (PB_CONFIG_FILE)")
	logLevel1 := flag.Bool("v", false, "Sets log level for ansible-shim to INFO (default WARN)")
	logLevel2 := flag.Bool("vv", false, "Sets log level for ansible-shim to DEBUG (default WARN)")
	flag.Parse()

	// display version/build info if -version was passed to CLI
	if *displayVersion {
		fmt.Printf("Version:\t%s\n", BuildVersion)
		fmt.Printf("Build date:\t%s\n", BuildDate)
		os.Exit(0)
	}

	// set log level accoring to -v / -vv CLI options
	if *logLevel1 {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}
	if *logLevel2 {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// playbook config
	pb := PlaybookConfig{}

	// read config file defined by flag or environment variable into PlaybookConfig struct
	err := pb.readConf(pbConfigFile)
	if err != nil {
		slog.Error("Exiting due to config file error: %s", err)
		os.Exit(1)
	}

	// read environment variables into PlaybookConfig struct
	err = pb.readEvs()
	if err != nil {
		slog.Error("Exiting due error reading environment variables: %s", err)
		os.Exit(1)
	}

	// validate inputs in PlaybookConfig struct
	err = pb.validateInputs()
	if err != nil {
		slog.Error("Exiting due valication errors: %s", err)
		os.Exit(1)
	}

	// Using PlaybookConfig struct, determine runtime environment (ansible in path, in Python venv, or container).
	// If container execution, write struct to file, run ansible-shim inside container to read and execute ansible.
	exitCode, err := pb.runAnsiblePlaybook()
	if err != nil {
		slog.Error("Error running playbook: %s", err)
		os.Exit(1)
	}

	// Final exit code is based on the results of above runAnsiblePlaybook method call
	os.Exit(exitCode)

}

// PlaybookConfig method to validate captured inputs from PlaybookConfig struct
func (c *PlaybookConfig) validateInputs() error {

	if c.VerboseLevel < 0 || c.VerboseLevel > 4 {
		slog.Warn("VERBOSE_LEVEL must be between 0 and 4, using default (1).")
		c.VerboseLevel = 1
	}

	if c.Playbook == "" {
		return &InputError{
			Err: errors.New("playbook is required"),
		}
	}

	err := sanitizePath(c.Playbook, false)
	slog.Info(fmt.Sprintf("Checking playbook path: %s", c.Playbook))
	if err != nil {
		slog.Error("Error validating playbook path")
		return err
	}

	if c.InventoryFile == "" {
		return &InputError{
			Err: errors.New("inventory parameter is required"),
		}
	}

	err = sanitizePath(c.InventoryFile, false)
	slog.Info(fmt.Sprintf("Checking inventory path: %s", c.InventoryFile))
	if err != nil {
		slog.Error("Error validating inventory path")
		return err
	}

	if c.ExtraVarsFile != "" {
		err := sanitizePath(c.ExtraVarsFile, false)
		if err != nil {
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
			slog.Error("Error validating path to SSH private key")
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
func (c *PlaybookConfig) readEvs() error {

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

	containerCmd := ""

	// Need to determine container runtime
	_, err := exec.LookPath("docker")
	if err == nil {
		slog.Info("found docker")
		containerCmd = "docker"
	}
	_, err = exec.LookPath("podman")
	if err == nil {
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
				fmt.Println(ctx.Err())
				return
			default:
				//fmt.Println("finished")
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
	if err != nil {
		slog.Error("cmd.Start() failed")
		return 1, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
	}
	if scanner.Err() != nil {
		cmd.Process.Kill()
		cmd.Wait()
		//panic(scanner.Err())
		return 1, scanner.Err()
	}

	rc := 0
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", err))
			rc = exiterr.ExitCode()
		}
	}

	slog.Info("container finished")
	return rc, err

}

// PlaybookConfig method to read PlaybookConfig struct, formulate and run ansible-playbook command
func (c *PlaybookConfig) runAnsiblePlaybook() (int, error) {

	var (
		err error
	)

	rc := 0

	// container image was specified
	if c.Image != "" {
		rc, err = executePlaybookInContainer(*c)
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
				fmt.Println(ctx.Err())
				return
			default:
				//fmt.Println("finished")
				cancel()
			}
		}
	}()

	slog.Debug(fmt.Sprintf("%s %s", ansibleCmdPath, ansiblePlaybookArgs))

	cmd := exec.CommandContext(ctx, ansibleCmdPath, ansiblePlaybookArgs...)

	slog.Info(cmd.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
		//panic(err)
	}
	scanner := bufio.NewScanner(stdout)
	slog.Info("about to start")
	err = cmd.Start()
	if err != nil {
		slog.Error("cmd.Start() failed")
		return 1, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
	}
	if scanner.Err() != nil {
		cmd.Process.Kill()
		cmd.Wait()
		//panic(scanner.Err())
		return 1, scanner.Err()
	}

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", err))
			rc = exiterr.ExitCode()
		}
	}

	slog.Info("ansible-playbook finished")
	return rc, err
}

func LookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func LookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("LookupEnvOrBool[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

func LookupEnvOrInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("LookupEnvOrInt[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}
