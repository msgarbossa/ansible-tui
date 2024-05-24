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

	tui.app.SetFocus(tui.listNav)
	tui.save() // save struct to file
}

func (tui *TUI) renderHeader() {

	// if nothing matches the switch statement below, display the default header
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
			"<a>", "show all", "", "", BuildDate)
	case "Playbooks":
		headerText = fmt.Sprintf(
			`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
	`, "<i>", "inspect", "<esc>", "back", "ansible-tui",
			"", "", "enter", "select", BuildVersion,
			"<a>", "show all", "", "", BuildDate)
	case "Inventory":
		headerText = fmt.Sprintf(
			`%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
%-7s %-10s %-7s %-10s %-10s
		`, "<i>", "inspect", "<esc>", "back", "ansible-tui",
			"<v>", "verify", "enter", "select", BuildVersion,
			"<a>", "show all", "", "", BuildDate)
	}

	// tui.textTop.SetText(fmt.Sprintf("version: %s\ndate: %s", BuildVersion, BuildDate))
	tui.textTop.SetText(headerText)
}

func (tui *TUI) renderFooter(msg string) {
	tui.textFooter.SetText(fmt.Sprintf("config: %s", msg))
}

func (tui *TUI) lintMenu() {
	tui.editParam = "Lint"

	// cmd := "ansible-lint"
	// cmdArgs := []string{"images"}
	// rc, outputSlice, err := cmd.RunBufferedCommand(containerCmd, containerCmdArgs, -1, true, "")

	// if err != nil {
	// 	output := strings.Join(*outputSlice, "\n") // convert slice of strings to string
	// 	output = fmt.Sprintf("rc: %d, stdout: %s, error: %v", rc, output, err)
	// 	slog.Error(output)
	// 	tui.pages.SwitchToPage("main text")
	// 	tui.textMain1.Clear()
	// 	tui.textMain1.SetTitle("Error")
	// 	tui.textMain1.SetText(output)
	// 	tui.app.SetFocus(tui.listNav)
	// 	tui.app.Sync()
	// 	return
	// }

	tui.renderHeader()
	tui.pages.SwitchToPage("main table")
	tui.tableMain.Clear()
	tui.tableMain.SetTitle("Lint Menu")
	tui.tableMain.SetSelectable(true, false)

	idx := 0
	tui.tableMain.SetCell(
		idx, 1,
		&tview.TableCell{
			Text:          "Lint entire repository",
			Color:         tcell.ColorYellow,
			NotSelectable: false,
		},
	)
	idx++
	tui.tableMain.SetCell(
		idx, 1,
		&tview.TableCell{
			Text:          "Lint currently selected playbook",
			Color:         tcell.ColorYellow,
			NotSelectable: false,
		},
	)

	tui.tableMain.ScrollToBeginning()
	// tui.tableMain.InputHandler()
	tui.app.SetFocus(tui.tableMain)
	tui.app.Sync() // without this, listing images corrupts the screen
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
			errStr := fmt.Sprintf("Input validation listing limits: %s", err)
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
	filter := true
	// make filtering optional using variable set by keyboard event handler
	if tui.editParam == "Playbooks-all" {
		filter = false
	}
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
	if !filter {
		yamlFiles, err = cmd.ListDir(tui.pbConfig.Tui.PlaybookDir, fileExt)
	} else if tui.pbConfig.Tui.PlaybookDir == "." {
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
		pbBool := true
		if filter {
			pbBool, err = cmd.IsPlaybookFile(line)
			if err != nil {
				slog.Error(fmt.Sprintf("Could not check if file is playbook: %s, %s", line, err))
			}
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

	tui.app.SetFocus(tui.tableMain)
	tui.tableMain.ScrollToBeginning()

	if idx == 0 {
		tui.tableMain.SetCell(
			idx, 1,
			&tview.TableCell{
				Text:          "No playbook files found, set playbook-dir in config",
				Color:         tcell.ColorYellow,
				NotSelectable: false,
			},
		)
		tui.app.SetFocus(tui.listNav)
	}

	tui.app.Sync() // without this, listing images corrupts the screen
}

func (tui *TUI) listInventoryFiles() {
	filter := true
	// make filtering optional using variable set by keyboard event handler
	if tui.editParam == "Inventory-all" {
		filter = false
	}
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

	slog.Debug(fmt.Sprintf("Checking directory for inventory files with yaml/yml, ini, txt extensions: %s", tui.pbConfig.Tui.InventoryDir))
	if !filter {
		yamlFiles, err = cmd.ListDir(tui.pbConfig.Tui.InventoryDir, []string{})
	} else if tui.pbConfig.Tui.InventoryDir == "." {
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
		pbBool, err := cmd.IsInventory(line)
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

	tui.app.SetFocus(tui.tableMain)
	tui.tableMain.ScrollToBeginning()

	if idx == 0 {
		tui.tableMain.SetCell(
			idx, 1,
			&tview.TableCell{
				Text:          "No inventory files found, set inventory-dir in config",
				Color:         tcell.ColorYellow,
				NotSelectable: false,
			},
		)
		tui.app.SetFocus(tui.listNav)
	}

	tui.app.Sync() // without this, listing images corrupts the screen
}

func tuiExecuteLint(c *cmd.PlaybookConfig, target string) {

	// Reset target based on string selected in TUI
	if strings.Contains(target, "all") {
		target = "."
	} else {
		target = c.Playbook
	}

	c.LintEnabled = true // this prevents SSH from being used

	// Need to process and validate inputs for environment setup (Python virtualenv, container, etc.)

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

	// Call lint method in cmd package, which will run ansible-lint in a container or python virtualenv
	c.Metrics.ExitCode, err = c.RunAnsibleLint(target)
	if err != nil {
		slog.Error(fmt.Sprintf("Error running lint: %s", err))
		os.Exit(1)
	}

	// Final exit code is based on the results of above RunAnsibleLint method call
	if err != nil {
		os.Exit(c.Metrics.ExitCode)
	}
	os.Exit(c.Metrics.ExitCode)
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
