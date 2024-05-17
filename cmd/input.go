package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

type TuiParams struct {
	PlaybookDir    string `yaml:"playbook-dir" json:"playbook-dir"`
	InventoryDir   string `yaml:"inventory-dir" json:"inventory-dir"`
	ImageFilter    string `yaml:"image-filter" json:"image-filter"`
	VirtualEnvsDir string `yaml:"virtual-envs-dir" json:"virtual-envs-dir"`
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
	PlaybookTimeout      int                          `yaml:"playbook-timeout" json:"playbook-timeout"`
	EnvironmentVariables PlaybookEnvironmentVariables `yaml:"environment-variables"`
	Metrics              PlaybookMetrics
	TempDirPath          string `yaml:"temp-dir-path" json:"temp-dir-path"`
	ConfigFilePath       string
	Tui                  TuiParams `yaml:"tui" json:"tui"`
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

// defaults set internally (not TempDirPath or ConfigFilePath)
func NewPlaybookConfig() *PlaybookConfig {
	return &PlaybookConfig{
		PlaybookTimeout: 86400,
		VerboseLevel:    1,
	}
}

func (c *PlaybookConfig) GenerateTemplateFile(defaultConfigFilePath string) error {

	// read environment variables into PlaybookConfig struct (also creates temp-dir)
	err := c.ReadEnvs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to error reading environment variables: %s", err))
		os.Exit(1)
	}

	playbookDir := "."
	if ok, _ := pathExists("./playbooks", true); ok {
		playbookDir = "./playbooks"
	}

	inventoryDir := "."
	if ok, _ := pathExists("./inventory", true); ok {
		inventoryDir = "./inventory"
	}

	contents := `---
# virtual-env-path: ""
image: ansible-tui:latest
ssh-private-key-file: "~/.ssh/id_rsa"
remote-user: root
inventory: "./examples/hosts.yml"
playbook: "./examples/site.yml"
verbose-level: 1

tui:
  playbook-dir: "` + playbookDir + `"
  inventory-dir: "` + inventoryDir + `"
  image-filter: ansible
  virtual-envs-dir: ""
`

	WriteFileFromString(defaultConfigFilePath, contents)

	return nil
}

func (c *PlaybookConfig) ReadEnvs() error {

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
		} else {
			c.VerboseLevel = verboseLevelInt
		}
		// TODO: should probably evaluate this right away in a separate function and set log level
	}

	sshPrivateKeyFile := os.Getenv("SSH_PRIVATE_KEY_FILE")
	if sshPrivateKeyFile != "" {
		c.SshPrivateKeyFile = sshPrivateKeyFile
	}

	remoteUser := os.Getenv("ANSIBLE_REMOTE_USER")
	if remoteUser != "" {
		c.RemoteUser = remoteUser
	}

	tmpDirPath := os.Getenv("TMP_DIR_PATH")
	if tmpDirPath != "" {
		c.TempDirPath = tmpDirPath
	}

	// create temp-dir if it does not exist
	p := c.TempDirPath
	if checkRelativePath(p) {
		cwd, _ := os.Getwd()
		p = filepath.Join(cwd, p)
	}
	if ok, _ := pathExists(p, true); !ok {
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
		err := WriteFileFromString(c.InventoryFile, inventoryContents)
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
		err := WriteFileFromString(c.ExtraVarsFile, extraVarsContents)
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

	playbookTimeout := os.Getenv("ANSIBLE_PLAYBOOK_TIMEOUT")
	if playbookTimeout != "" {
		c.PlaybookTimeout, err = strconv.Atoi(playbookTimeout)
		if err != nil {
			slog.Error("Could not convert ANSIBLE_PLAYBOOK_TIMEOUT to integer")
			return err
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

func (c *PlaybookConfig) ReadConf(pbConfigFile string) error {

	slog.Debug(fmt.Sprintf("Reading config file: %s", pbConfigFile))

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

func (c *PlaybookConfig) ProcessEnvs() error {

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

	// TODO: Should auto-pass variables with ANSIBLE_* prefix?  Optional?  Global config file?

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

// PlaybookConfig method to validate captured inputs from PlaybookConfig struct
func (c *PlaybookConfig) ValidateInputs() error {

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
		slog.Warn("Container image was specified, unsetting virtual environment")
		c.VirtualEnvPath = ""
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
