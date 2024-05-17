package tui

import (
	"a5e/cmd"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
)

func (tui *TUI) save() {

	// write PlaybookConfig struct to file

	// Make a copy of config to manipulate before writing to file
	writeConfig := tui.toWriteConfig()

	b, err := yaml.Marshal(&writeConfig)
	if err != nil {
		slog.Error("Could not marshal PlaybookConfig to bytes")
	}

	err = os.WriteFile(tui.pbConfig.ConfigFilePath, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", tui.pbConfig.ConfigFilePath))
	}

	tui.pages.SwitchToPage("main text")
	tui.textMain1.SetText(fmt.Sprintf("Wrote config to %s:\n\n%s", tui.pbConfig.ConfigFilePath, b))
}

func (tui *TUI) saveAdvancedForm() {

	// Get verbose-level from form and convert to integer
	verboseString := tui.formAdvanced.GetFormItemByLabel("verbose-level").(*tview.InputField).GetText()
	verboseInt, err := strconv.Atoi(verboseString)
	if err != nil {
		slog.Error(fmt.Sprintf("Could not convert verbose-level to int: %s", verboseString))
	} else {
		tui.pbConfig.VerboseLevel = verboseInt
	}

	// Get the rest of the form values
	tui.pbConfig.RemoteUser = tui.formAdvanced.GetFormItemByLabel("remote-user").(*tview.InputField).GetText()
	tui.pbConfig.SshPrivateKeyFile = tui.formAdvanced.GetFormItemByLabel("ssh-private-key-file").(*tview.InputField).GetText()
	tui.pbConfig.VirtualEnvPath = tui.formAdvanced.GetFormItemByLabel("virtual-env-path").(*tview.InputField).GetText()
	tui.pbConfig.WindowsGroup = tui.formAdvanced.GetFormItemByLabel("windows-group").(*tview.InputField).GetText()
	tui.pbConfig.Tui.PlaybookDir = tui.formAdvanced.GetFormItemByLabel("playbook-dir").(*tview.InputField).GetText()
	tui.pbConfig.Tui.InventoryDir = tui.formAdvanced.GetFormItemByLabel("inventory-dir").(*tview.InputField).GetText()
	tui.pbConfig.Tui.ImageFilter = tui.formAdvanced.GetFormItemByLabel("image-filter").(*tview.InputField).GetText()

	tui.save() // save struct to file
}

func (tui *TUI) renderHeader() {

	headerText := fmt.Sprintf(
		`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
`, "", "", "", "", "ansible-tui",
		"", "", "", "", BuildVersion,
		"", "", "", "", BuildDate)

	switch tui.editParam {
	case "Images":
		headerText = fmt.Sprintf(
			`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
	`, "<i>", "inspect", "<esc>", "back", "ansible-tui",
			"<c>", "clear", "enter", "select", BuildVersion,
			"", "", "", "", BuildDate)
	case "Playbooks":
		headerText = fmt.Sprintf(
			`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
	`, "<i>", "inspect", "<esc>", "back", "ansible-tui",
			"", "", "enter", "select", BuildVersion,
			"", "", "", "", BuildDate)
	case "Inventory":
		headerText = fmt.Sprintf(
			`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
		`, "<i>", "inspect", "<esc>", "back", "ansible-tui",
			"<v>", "verify", "enter", "select", BuildVersion,
			"", "", "", "", BuildDate)
	}

	// tui.textTop.SetText(fmt.Sprintf("version: %s\ndate: %s", BuildVersion, BuildDate))
	tui.textTop.SetText(headerText)
}

func (tui *TUI) renderFooter(msg string) {
	tui.textFooter.SetText(fmt.Sprintf("config: %s", msg))
}

func (tui *TUI) listImages() {
	tui.editParam = "Images"

	containerCmd, err := cmd.GetContainerRuntime()
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	}
	// containerCmd = "cat"
	containerCmdArgs := []string{"images"}
	rc, outputSlice, err := cmd.RunBufferedCommand(containerCmd, containerCmdArgs, -1, true, "")

	if err != nil {
		output := strings.Join(*outputSlice, "\n") // convert slice of strings to string
		output = fmt.Sprintf("rc: %d, stdout: %s, error: %v", rc, output, err)
		slog.Error(output)
		tui.pages.SwitchToPage("main text")
		tui.textMain1.Clear()
		tui.textMain1.SetTitle("Error")
		tui.textMain1.SetText(output)
		tui.app.SetFocus(tui.listNav)
		tui.app.Sync()
		return
	}

	tui.renderHeader()
	tui.pages.SwitchToPage("main table")
	tui.tableMain.Clear()
	tui.tableMain.SetTitle("Images")
	tui.tableMain.SetSelectable(true, false)

	idx := 0
	for _, line := range *outputSlice {
		// if image filter is set, only show images that contain the filter string
		if tui.pbConfig.Tui.ImageFilter != "" {
			if !strings.Contains(line, tui.pbConfig.Tui.ImageFilter) {
				continue
			}
		}

		// skip empty lines
		if line == "" {
			continue
		}
		tui.tableMain.SetCell(
			idx, 1,
			&tview.TableCell{
				Text:          line,
				Color:         tcell.ColorYellow,
				NotSelectable: false,
			},
		)
		idx++
	}

	tui.tableMain.ScrollToBeginning()
	// tui.tableMain.InputHandler()
	tui.app.SetFocus(tui.tableMain)
	tui.app.Sync() // without this, listing images corrupts the screen
}

func parseImageShort(image string) string {
	// Parse the full image name into short name and tag
	if strings.Contains(image, "/") {
		imageParts := strings.Split(image, "/")
		image = imageParts[len(imageParts)-1] // get last index from imageParts
	}
	return image
}

func inspectImage(image string) *string {
	containerCmd, err := cmd.GetContainerRuntime()
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	}

	out, err := exec.Command(containerCmd, "inspect", image).Output()
	if err != nil {
		log.Fatal(err)
	}
	outStr := string(out)

	// TODO: parse the output and create a header summarizing key metadata / bill-of-materials

	return &outStr
}

func inspectFile(file string) *string {
	b, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	s := string(b)
	return &s
}

func isInventory(file string) (bool, error) {
	// Read file and check if it does NOT contains "become:"
	// Checking inventory is more difficult to do efficiently.
	// In this quick check, simple pattern match rules are used to score the file
	// to determine if it should be displayed in the list.
	// It really should run through ansible-inventory.
	// To do this, the keyboard shortcuts can be used to inspect or verify the inventory file.

	// read the whole file at once
	b, err := os.ReadFile(file)
	if err != nil {
		return false, err
	}
	s := string(b) // convert bytes to string

	found := false
	score := 0
	// check for typical inventory file patterns
	if !strings.Contains(s, "become:") {
		score++
	}
	if strings.Contains(s, "children:") {
		score += 5
	}
	if score > 0 {
		found = true
	}

	return found, nil
}

func isPlaybookFile(file string) (bool, error) {
	// Read file and check if it contains "become:"

	// read the whole file at once
	b, err := os.ReadFile(file)
	if err != nil {
		return false, err
	}
	s := string(b) // convert bytes to string
	// check whether s contains substring text
	found := strings.Contains(s, "become:")
	return found, nil
}

func (tui *TUI) listLimits() {
	tui.editParam = "Limits"

	tui.renderHeader()
	tui.pages.SwitchToPage("main table")
	tui.tableMain.Clear()
	tui.tableMain.SetTitle("Limits from selected inventory")
	tui.tableMain.SetSelectable(true, false)

	var limitList []string
	var err error

	// process values in PlaybookConfig struct
	err = tui.pbConfig.ProcessEnvs()
	if err != nil {
		errStr := fmt.Sprintf("Error processing inputs: %s", err)
		slog.Error(errStr)
		limitList = append(limitList, errStr)
		tui.tableMain.SetSelectable(false, false)
	}

	// validate inputs in PlaybookConfig struct
	if err == nil {
		err = tui.pbConfig.ValidateInputs()
		if err != nil {
			errStr := fmt.Sprintf("Input validation: %s", err)
			slog.Error(errStr)
			limitList = append(limitList, errStr)
			tui.tableMain.SetSelectable(false, false)
		}
	}

	if err == nil {
		output, err := tui.pbConfig.GetAnsibleInventory(tui.pbConfig.InventoryFile)
		if err != nil {
			errStr := fmt.Sprintf("Error running ansible-inventory: %s", err)
			slog.Error(errStr)
			limitList = append(limitList, errStr)
			tui.tableMain.SetSelectable(false, false)
		} else {
			limitList = append(limitList, *output...)
		}
	}

	for idx, line := range limitList {
		tui.tableMain.SetCell(
			idx, 1,
			&tview.TableCell{
				Text:          line,
				Color:         tcell.ColorYellow,
				NotSelectable: false,
			},
		)
	}

	tui.tableMain.ScrollToBeginning()
	tui.app.SetFocus(tui.tableMain)
	tui.app.Sync() // without this, listing images corrupts the screen
}

func (tui *TUI) listPlaybooks() {
	tui.editParam = "Playbooks"

	tui.renderHeader()
	tui.pages.SwitchToPage("main table")
	tui.tableMain.Clear()
	tui.tableMain.SetTitle("Playbooks")
	tui.tableMain.SetSelectable(true, false)

	// TODO: Print any errors to the UI.
	var yamlFiles []string
	var err error
	fileExt := []string{"yaml", "yml"}

	slog.Debug(fmt.Sprintf("Checking directory for playbook files with yaml extension: %s", tui.pbConfig.Tui.PlaybookDir))
	if tui.pbConfig.Tui.PlaybookDir == "." {
		yamlFiles, err = cmd.ListDir(tui.pbConfig.Tui.PlaybookDir, fileExt)
	} else {
		yamlFiles, err = cmd.WalkDir(tui.pbConfig.Tui.PlaybookDir, fileExt)
	}
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	}

	sort.Strings(yamlFiles)

	idx := 0
	for _, line := range yamlFiles {
		pbBool, err := isPlaybookFile(line)
		if err != nil {
			slog.Error(fmt.Sprintf("Could not check if file is playbook: %s, %s", line, err))
		}
		if pbBool {
			tui.tableMain.SetCell(
				idx, 1,
				&tview.TableCell{
					Text:          line,
					Color:         tcell.ColorYellow,
					NotSelectable: false,
				},
			)
			idx++
		}
	}

	tui.tableMain.ScrollToBeginning()
	tui.app.SetFocus(tui.tableMain)
	tui.app.Sync() // without this, listing images corrupts the screen
}

func (tui *TUI) listInventoryFiles() {
	tui.editParam = "Inventory"

	tui.renderHeader()
	tui.pages.SwitchToPage("main table")
	tui.tableMain.Clear()
	tui.tableMain.SetTitle("Inventory")
	tui.tableMain.SetSelectable(true, false)

	// TODO: Print any errors to the UI.
	var yamlFiles []string
	var err error
	fileExt := []string{"yaml", "yml", "ini", "txt", ""}

	slog.Info(fmt.Sprintf("Checking directory for inventory files with yaml/yml, ini, txt extensions: %s", tui.pbConfig.Tui.InventoryDir))
	if tui.pbConfig.Tui.InventoryDir == "." {
		yamlFiles, err = cmd.ListDir(tui.pbConfig.Tui.InventoryDir, fileExt)
	} else {
		yamlFiles, err = cmd.WalkDir(tui.pbConfig.Tui.InventoryDir, fileExt)
	}
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
	}

	sort.Strings(yamlFiles)

	idx := 0
	for _, line := range yamlFiles {
		pbBool, err := isInventory(line)
		if err != nil {
			slog.Error(fmt.Sprintf("Could not check if file is inventory: %s, %s", line, err))
		}
		if pbBool {
			tui.tableMain.SetCell(
				idx, 1,
				&tview.TableCell{
					Text:          line,
					Color:         tcell.ColorYellow,
					NotSelectable: false,
				},
			)
			idx++
		}
	}
	tui.tableMain.ScrollToBeginning()
	tui.app.SetFocus(tui.tableMain)
	tui.app.Sync() // without this, listing images corrupts the screen
}

func tuiExecutePlaybook(c *cmd.PlaybookConfig) {

	// The code below is copied from the last part of main.go.
	// Prior to this function call, the following were executed to stop TUI.
	// tui.flex.Clear()
	// tui.app.Sync()
	// tui.Stop()

	// process values in PlaybookConfig struct
	err := c.ProcessEnvs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to errors processing inputs: %s", err))
		os.Exit(1)
	}

	// validate inputs in PlaybookConfig struct
	err = c.ValidateInputs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due validation errors: %s", err))
		os.Exit(1)
	}

	// Using PlaybookConfig struct, determine runtime environment (ansible in path, in Python venv, or container).
	// If container execution, write struct to file, run ansible-tui inside container to read and execute ansible.
	c.Metrics.ExitCode, err = c.RunAnsiblePlaybook()
	if err != nil {
		slog.Error(fmt.Sprintf("Error running playbook: %s", err))
		os.Exit(1)
	}

	// Final exit code is based on the results of above RunAnsiblePlaybook method call
	if err != nil {
		os.Exit(c.Metrics.ExitCode)
	}
	os.Exit(c.Metrics.ExitCode)

}
