package cmd

// TODO: This all has more to do with the TUI and should probably go in the tui package.
//       Also some of the helpers like IsPlaybookFile.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Settings in this struct set initial default values for new PlaybookConfig structs.
type GlobalConfigDefault struct {
	RemoteUser string `yaml:"remote-user" json:"remote-user"`
	Image      string `yaml:"image" json:"image"`
}

// This configuration file is read in from /etc/ansible/ansible-tui-force-config.yml.
// Settings in this struct can be globally enforced (not available for user input).
// This can be used for governance such as lint rules or logging.
type GlobalConfigForce struct {
	AnsibleLintFilePath string `yaml:"ansible-lint-file-path" json:"ansible-lint-file-path"`
	AnsibleLintFileUrl  string `yaml:"ansible-lint-file-url" json:"ansible-lint-file-url"`
}

// This configuration file is read in from /etc/ansible/ansible-tui-config.yml.
type GlobalConfig struct {
	Defaults GlobalConfigDefault `yaml:"default" json:"default"`
	Force    GlobalConfigForce   `yaml:"force" json:"force"`
}

var (
	ansibleTuiGlobalConfigPath string = "/etc/ansible/ansible-tui-config.yml"
)

func (c *GlobalConfigDefault) toMAP() (res map[string]interface{}) {
	a, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(a, &res)
	return
}

func (c *PlaybookConfig) toMAP() (res map[string]interface{}) {
	a, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(a, &res)
	return
}

func findPlaybookDir() string {
	if ok, _ := pathExists("./playbooks", true); ok {
		return "./playbooks"
	}

	// Look in current directory for YAML files.
	// Immediately use directory if site.yml or site.yaml is found.
	// Otherwise keep looking for playbooks with another name.
	fileExt := []string{"yaml", "yml"}
	yamlFiles, err := ListDir(".", fileExt)
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	} else {
		for _, line := range yamlFiles {
			if line == "site.yml" || line == "site.yaml" {
				return "."
			}
			pbBool, err := IsPlaybookFile(line)
			if err != nil {
				slog.Debug(fmt.Sprintf("Could not check if file is playbook: %s, %s", line, err))
			}
			if pbBool {
				slog.Debug(fmt.Sprintf("Found playbook directory based on file: %s", line))
				return "."
			}
		}
	}

	// Recurse into directories looking for playbooks
	// TODO: Should skip files already inspected above, but it wouldn't save much time.
	yamlFiles, err = WalkDir(".", fileExt)
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	} else {
		for _, line := range yamlFiles {
			if line == "site.yml" || line == "site.yaml" {
				slog.Debug(fmt.Sprintf("Found playbook directory based on named file: %s", line))
				return "./" + filepath.Dir(line)
			}
			pbBool, err := IsPlaybookFile(line)
			if err != nil {
				slog.Debug(fmt.Sprintf("Could not check if file is playbook: %s, %s", line, err))
			}
			if pbBool {
				slog.Debug(fmt.Sprintf("Found playbook directory based on file: %s", line))
				return "./" + filepath.Dir(line)
			}
		}
	}

	slog.Debug("No playbooks found so returning current directory")
	return "."
}

func findInventoryDir() string {
	if ok, _ := pathExists("./inventory", true); ok {
		slog.Debug("Found inventory directory: ./inventory")
		return "./inventory"
	}

	// Look in current directory for various file extensions.
	fileExt := []string{"yaml", "yml", "ini", "txt", ""}
	yamlFiles, err := ListDir(".", fileExt)
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	} else {
		for _, line := range yamlFiles {
			pbBool, err := IsPlaybookFile(line)
			if err != nil {
				slog.Debug(fmt.Sprintf("Could not check if file is inventory: %s, %s", line, err))
			}
			if pbBool {
				slog.Debug(fmt.Sprintf("Found inventory directory based on file: %s", line))
				return "."
			}
		}
	}

	// Recurse into directories looking for inventory.
	// TODO: Should skip files already inspected above, but it wouldn't save much time.
	yamlFiles, err = WalkDir(".", fileExt)
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	} else {
		for _, line := range yamlFiles {
			// slog.Debug(fmt.Sprintf("Checking if file is inventory: %s", line))
			pbBool, err := IsInventory(line)
			if err != nil {
				slog.Debug(fmt.Sprintf("Could not check if file is inventory: %s, %s", line, err))
			}
			if pbBool {
				slog.Debug(fmt.Sprintf("Found inventory directory based on file: %s", line))
				return "./" + filepath.Dir(line)
			}
		}
	}

	slog.Debug("No inventory files found so returning current directory")
	return "."
}

func (c *PlaybookConfig) GenerateTemplateFile(defaultConfigFilePath string) error {

	// read environment variables into PlaybookConfig struct (also creates temp-dir)
	err := c.ReadEnvs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to error reading environment variables: %s", err))
		os.Exit(1)
	}

	playbookDir := findPlaybookDir()

	inventoryDir := findInventoryDir()

	// If ansibleTuiGlobalConfigPath exists, read it in and merge with new values
	// TODO: Make this cleaner.  Merging maps is easy, but not structs.
	if ok, _ := pathExists(ansibleTuiGlobalConfigPath, false); ok {
		d, err := ReadGlobalConfig()
		if err == nil {
			// unmarshal global defaults into map
			dMap := d.Defaults.toMAP()
			// unmarshal current config into map
			cMap := c.toMAP()
			// merge maps (overwriting values in Playbook/cMap)
			for k, v := range dMap {
				cMap[k] = v
			}
			// Marshal merged map back into PlaybookConfig struct
			cTemp, _ := json.Marshal(cMap)
			_ = json.Unmarshal(cTemp, &c)
		}
	}

	contents := `---
# virtual-env-path: ""
image: "` + c.Image + `"
ssh-private-key-file: "~/.ssh/id_rsa"
remote-user: "` + c.RemoteUser + `"
inventory: ""
playbook: ""
verbose-level: 0

tui:
  playbook-dir: "` + playbookDir + `"
  inventory-dir: "` + inventoryDir + `"
  image-filter: ansible
  virtual-envs-dir: ""
`

	WriteFileFromString(defaultConfigFilePath, contents)

	return nil
}

func ReadGlobalConfig() (*GlobalConfig, error) {

	var (
		err error
	)

	buf, err := os.ReadFile(ansibleTuiGlobalConfigPath)
	if err != nil {
		slog.Error(fmt.Sprintf("Error reading global defaults config file %s: %s", ansibleTuiGlobalConfigPath, err))
		return nil, err
	}

	c := &GlobalConfig{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		slog.Error(fmt.Sprintf("Error unmarshalling global defaults config file %s: %s", ansibleTuiGlobalConfigPath, err))
		return nil, err
	}

	return c, nil
}
