package tui

import (
	"a5e/cmd"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"log/slog"

	"github.com/rivo/tview"
)

// global variables
var (
	BuildVersion string = ""
	BuildDate    string = ""
)

// TUI implement terminal user interface features.
// It also provides easy-to-use, easy-to-access abstraction over underlying tview components.
type TUI struct {
	// Internal structures.
	pbConfig *cmd.PlaybookConfig

	// View components.
	app          *tview.Application
	textMain1    *tview.TextView
	textDetail1  *tview.TextView
	textFooter   *tview.TextView
	tableMain    *tview.Table
	formAdvanced *tview.Form
	listNav      *tview.List
	textTop      *tview.TextView
	flex         *tview.Flex
	pages        *tview.Pages
	editParam    string
}

type playbookEnvironmentVariables struct {
	Pass []string          `json:"pass"`
	Set  map[string]string `json:"set"`
}

type tuiParams struct {
	PlaybookDir    string `yaml:"playbook-dir" json:"playbook-dir"`
	InventoryDir   string `yaml:"inventory-dir" json:"inventory-dir"`
	ImageFilter    string `yaml:"image-filter" json:"image-filter"`
	VirtualEnvsDir string `yaml:"virtual-envs-dir" json:"virtual-envs-dir"`
}
type writeConfig struct {
	Playbook             string                       `yaml:"playbook" json:"playbook"`
	InventoryFile        string                       `yaml:"inventory" json:"inventory"`
	LimitHost            string                       `yaml:"limit" json:"limit"`
	Image                string                       `yaml:"image" json:"image"`
	VerboseLevel         int                          `yaml:"verbose-level" json:"verbose-level"`
	SshPrivateKeyFile    string                       `yaml:"ssh-private-key-file" json:"ssh-private-key-file"`
	RemoteUser           string                       `yaml:"remote-user" json:"remote-user"`
	ExtraVarsFile        string                       `yaml:"extra-vars-file" json:"extra-vars-file"`
	AnsibleTags          string                       `yaml:"tags" json:"tags"`
	AnsibleSkipTags      string                       `yaml:"skip-tags" json:"skip-tags"`
	ExtraArgs            string                       `yaml:"extra-args" json:"extra-args"`
	WindowsGroup         string                       `yaml:"windows-group" json:"windows-group"`
	ExecutionType        string                       `yaml:"execution-type" json:"execution-type"`
	VirtualEnvPath       string                       `yaml:"virtual-env-path" json:"virtual-env-path"`
	PlaybookTimeout      int                          `yaml:"playbook-timeout" json:"playbook-timeout"`
	EnvironmentVariables playbookEnvironmentVariables `yaml:"environment-variables"`
	Tui                  tuiParams                    `yaml:"tui" json:"tui"`
}

func (tui *TUI) toWriteConfig() writeConfig {

	wc := writeConfig{
		Playbook:             tui.pbConfig.Playbook,
		InventoryFile:        tui.pbConfig.InventoryFile,
		LimitHost:            tui.pbConfig.LimitHost,
		Image:                tui.pbConfig.Image,
		VerboseLevel:         tui.pbConfig.VerboseLevel,
		SshPrivateKeyFile:    tui.pbConfig.SshPrivateKeyFile,
		RemoteUser:           tui.pbConfig.RemoteUser,
		ExtraVarsFile:        tui.pbConfig.ExtraVarsFile,
		AnsibleTags:          tui.pbConfig.AnsibleTags,
		AnsibleSkipTags:      tui.pbConfig.AnsibleSkipTags,
		ExtraArgs:            tui.pbConfig.ExtraArgs,
		WindowsGroup:         tui.pbConfig.WindowsGroup,
		ExecutionType:        tui.pbConfig.ExecutionType,
		VirtualEnvPath:       tui.pbConfig.VirtualEnvPath,
		PlaybookTimeout:      tui.pbConfig.PlaybookTimeout,
		EnvironmentVariables: playbookEnvironmentVariables(tui.pbConfig.EnvironmentVariables),
		Tui:                  tuiParams(tui.pbConfig.Tui),
	}

	return wc
}

func (tui *TUI) toMainMenu() {
	tui.textMain1.Clear()
	tui.pages.SwitchToPage("main text")
	tui.app.SetFocus(tui.listNav)
	tui.app.Sync()
}

func (tui *TUI) showAdvanced() {
	tui.pages.SwitchToPage("form advanced")
	tui.app.SetFocus(tui.formAdvanced)
	tui.app.Sync()
}

func (tui *TUI) mainMenu(c *cmd.PlaybookConfig) *tview.List {

	list := tview.NewList()
	list.SetBorder(true).SetTitle("Main Menu")
	list.AddItem("Inventory", c.InventoryFile, 'i', func() { tui.listInventoryFiles() }).
		AddItem("Playbook", c.Playbook, 'p', func() { tui.listPlaybooks() }).
		AddItem("Limit", c.LimitHost, 'l', func() { tui.listLimits() }).
		AddItem("Image", c.Image, 'I', func() { tui.listImages() }).
		AddItem("Advanced", "", 'a', func() { tui.showAdvanced() }).
		AddItem("Save", "", 's', func() { tui.save() }).
		AddItem("Lint", "", 'L', func() { tui.lintMenu() }).
		AddItem("Run", "", 'r', func() {
			tui.flex.Clear()
			tui.app.Sync()
			tui.Stop()
			tuiExecutePlaybook(c)
		}).
		AddItem("Quit", "", 'q', func() {
			tui.Stop()
			os.Exit(0)
		})

	return list
}

func (tui *TUI) setParam(key string, value string) {
	switch key {
	case "Inventory":
		tui.listNav.SetItemText(0, key, value)
	case "Playbook":
		tui.listNav.SetItemText(1, key, value)
	case "Limit":
		tui.listNav.SetItemText(2, key, value)
	case "Image":
		tui.listNav.SetItemText(3, key, value)
	case "Advanced":
		tui.listNav.SetItemText(4, key, value)
	}
}

func (tui *TUI) handleInspectTableCell(row int, _ int) {

	cell := tui.tableMain.GetCell(row, 1).Text
	var inspect *string

	switch tui.editParam {
	case "Images":
		fields := strings.Fields(cell)
		imageVal := fmt.Sprintf("%s:%s", fields[0], fields[1])
		inspect = inspectImage(imageVal)
		tui.textDetail1.SetText(*inspect)
		tui.textDetail1.ScrollToBeginning()
		tui.pages.SwitchToPage("detail text")
		tui.app.SetFocus(tui.textDetail1)
		tui.app.Sync()
	case "Playbooks":
		fields := strings.Fields(cell)
		playbookVal := fields[0]
		inspect = inspectFile(playbookVal)
		tui.textDetail1.SetText(*inspect)
		tui.textDetail1.ScrollToBeginning()
		tui.pages.SwitchToPage("detail text")
		tui.app.SetFocus(tui.textDetail1)
		tui.app.Sync()
	case "Inventory":
		fields := strings.Fields(cell)
		inventoryVal := fields[0]
		inspect = inspectFile(inventoryVal)
		tui.textDetail1.SetText(*inspect)
		tui.textDetail1.ScrollToBeginning()
		tui.pages.SwitchToPage("detail text")
		tui.app.SetFocus(tui.textDetail1)
		tui.app.Sync()
	}

}

func verifyInventoryFile(c *cmd.PlaybookConfig, invFilePath string) *string {
	// Run ansible-inventory --graph on the inventory file and return the output

	verifyOutput := fmt.Sprintf("ansible-inventory -i %s --graph:\n", invFilePath)

	// process values in PlaybookConfig struct
	err := c.ProcessEnvs()
	if err != nil {
		errStr := fmt.Sprintf("Error processing inputs: %s", err)
		slog.Error(errStr)
		verifyOutput += errStr + "\n"
	}

	// validate inputs in PlaybookConfig struct
	if err == nil {
		err = c.ValidateInputs()
		if err != nil {
			errStr := fmt.Sprintf("Validation errors: %s", err)
			slog.Error(errStr)
			verifyOutput += errStr + "\n"
		}
	} else {
		verifyOutput += "Skipping input validation due to previous error\n"
	}

	if err == nil {
		output, err := c.GetAnsibleInventory(invFilePath)
		if err != nil {
			errStr := fmt.Sprintf("Error running ansible-inventory: %s", err)
			slog.Error(errStr)
			verifyOutput += errStr + "\n"
		}
		verifyOutput += strings.Join(*output, "\n")
	} else {
		verifyOutput += "Skipping ansible-inventory due to previous error\n"
	}

	return &verifyOutput
}

func (tui *TUI) handleVerifyTableCell(row int, col int) {

	cell := tui.tableMain.GetCell(row, 1).Text

	switch tui.editParam {
	case "Inventory":
		fields := strings.Fields(cell)
		inventoryVal := fields[0]
		verifyOutput := verifyInventoryFile(tui.pbConfig, inventoryVal)
		slog.Debug(*verifyOutput)

		tui.textDetail1.SetText(*verifyOutput)
		tui.textDetail1.ScrollToBeginning()
		tui.pages.SwitchToPage("detail text")
		tui.app.SetFocus(tui.textDetail1)
		tui.app.Sync()
	}

}

func parseLimitEntry(limit string) string {

	// This function serves as an abstraction to the EvaluateInventoryGraphEntry function,  which uses
	// regex to grab the host/group from a single line and determines if the entry is a host or group
	// based on the @ symbol.  The TUI functionality could be expanded to handle more advanced --limit
	// options such as selecting multiple hosts or groups.

	// Example input will contain a line such as the ones in the following output
	// @all:
	//   |--@ungrouped:
	//   |  |--localhost

	// Specifying "all" is the same as not having a limit ("")
	if limit == "@all:" {
		return ""
	}
	newLimit, _, _ := cmd.EvaluateInventoryGraphEntry(limit)
	return newLimit
}

func (tui *TUI) handleSelectedTableCell(row int, col int) {

	cell := tui.tableMain.GetCell(row, 1).Text

	// TODO: make this more DRY
	switch tui.editParam {
	case "Images":
		fields := strings.Fields(cell)
		imageVal := fmt.Sprintf("%s:%s", fields[0], fields[1])
		imageValShort := parseImageShort(imageVal)
		tui.setParam("Image", imageValShort)
		tui.pbConfig.Image = imageVal
		tui.pages.SwitchToPage("main content")
		tui.textMain1.Clear()
		tui.app.SetFocus(tui.listNav)
		tui.app.Sync()
	case "Playbooks":
		fields := strings.Fields(cell)
		playbookVal := fields[0]
		tui.setParam("Playbook", playbookVal)
		tui.pbConfig.Playbook = playbookVal
		tui.pages.SwitchToPage("main content")
		tui.textMain1.Clear()
		tui.app.SetFocus(tui.listNav)
		tui.app.Sync()
	case "Inventory":
		fields := strings.Fields(cell)
		inventoryVal := fields[0]
		tui.setParam("Inventory", inventoryVal)
		tui.pbConfig.InventoryFile = inventoryVal
		tui.pages.SwitchToPage("main content")
		tui.textMain1.Clear()
		tui.app.SetFocus(tui.listNav)
		tui.app.Sync()
	case "Limits":
		fields := strings.Fields(cell)
		limitVal := parseLimitEntry(strings.Join(fields, " "))
		tui.setParam("Limit", limitVal)
		tui.pbConfig.LimitHost = limitVal
		tui.pages.SwitchToPage("main content")
		tui.textMain1.Clear()
		tui.app.SetFocus(tui.listNav)
		tui.app.Sync()
	case "Lint":
		fields := strings.Fields(cell)
		lintType := fields[0]
		tui.flex.Clear()
		tui.app.Sync()
		tui.Stop()
		tuiExecuteLint(tui.pbConfig, lintType)
		// tui.pages.SwitchToPage("main text")
		// tui.textMain1.Clear()
		// tui.app.SetFocus(tui.textMain1)
		// tui.textMain1.SetText(fmt.Sprintf("Running %s lint...", lintType))
		// tui.app.Sync()
	}

}

// NewTUI configures and returns an instance of terminal user interface.
func NewTUI(c *cmd.PlaybookConfig) (*TUI, error) {
	t := TUI{}
	t.pbConfig = c

	// Set log file for TUI
	tuiLogFile := filepath.Join(c.TempDirPath, "tui-last.log")
	fo, err := os.Create(tuiLogFile)
	if err != nil {
		slog.Error("opening file: %v", err)
	}
	// close fo on exit and check for its returned error
	defer func() {
		if err := fo.Close(); err != nil {
			panic(err)
		}
	}()
	logger := slog.New(slog.NewTextHandler(fo, nil))
	slog.SetDefault(logger)

	t.app = tview.NewApplication()

	// The top row has shortcuts relating to the current view
	t.textTop = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	fmt.Fprintf(t.textTop, "%s ", "Shortcuts relating to current view")
	t.textTop.SetBorder(true)
	// t.textTop.SetBorder(true).SetTitle("Top")

	t.listNav = t.mainMenu(c)
	// listNav.SetChangedFunc(setResponse)

	// The main content textView (will be page index 0)
	t.textMain1 = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	t.textMain1.SetBorder(true).SetTitle("")
	t.textMain1.ScrollToEnd()
	t.textMain1.SetChangedFunc(func() {
		t.app.Sync() // without this, listing images corrupts the screen
	})

	// The footer textView will be at the bottom of the screen to show status messages
	t.textFooter = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	t.textFooter.SetBorder(true)

	// The main content tableView (wil be page index 1)
	t.tableMain = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)
	t.tableMain.SetSelectedFunc(t.handleSelectedTableCell)

	// The detail content textView (will be page index 2)
	t.textDetail1 = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	t.textDetail1.SetBorder(true).SetTitle("Details")
	t.textDetail1.ScrollToEnd()
	t.textDetail1.SetChangedFunc(func() {
		t.app.Sync() // without this, listing images corrupts the screen
	})

	t.formAdvanced = tview.NewForm().
		// AddDropDown("verbose-level", []string{"0", "1", "2", "3", "4"}, 1, nil).
		AddTextView("Note", "The settings below and additional settings such as environment variables can also be modified by editing "+c.ConfigFilePath, 60, 3, true, false).
		AddInputField("verbose-level", fmt.Sprintf("%d", c.VerboseLevel), 2, nil, nil).
		AddInputField("remote-user", c.RemoteUser, 20, nil, nil).
		AddInputField("ssh-private-key-file", c.SshPrivateKeyFile, 60, nil, nil).
		AddInputField("virtual-env-path", c.VirtualEnvPath, 60, nil, nil).
		AddInputField("windows-group", c.WindowsGroup, 20, nil, nil).
		AddTextView("TUI configurations:", "These affect the behavior of the main menu.", 40, 2, true, false).
		AddInputField("playbook-dir", c.Tui.PlaybookDir, 30, nil, nil).
		AddInputField("inventory-dir", c.Tui.InventoryDir, 30, nil, nil).
		AddInputField("image-filter", c.Tui.ImageFilter, 20, nil, nil).
		AddButton("Cancel", func() { t.toMainMenu() }).
		AddButton("Save", func() { t.saveAdvancedForm() })
	t.formAdvanced.SetBorder(true).SetTitle("Advanced Configuration Options").SetTitleAlign(tview.AlignLeft)

	// modal := tview.NewModal().
	// 	SetText("Do you want to quit the application?").
	// 	AddButtons([]string{"Quit", "Cancel"}).
	// 	SetDoneFunc(func(buttonIndex int, buttonLabel string) {
	// 		if buttonLabel == "Quit" {
	// 			app.Stop()
	// 		}
	// 	})
	// if err := app.SetRoot(modal, false).SetFocus(modal).Run(); err != nil {
	// 	panic(err)
	// }

	// Use pages for main content area so widgets can be switched
	t.pages = tview.NewPages()
	t.pages.AddPage("main text", t.textMain1, true, true)
	t.pages.AddPage("main table", t.tableMain, true, false)
	t.pages.AddPage("detail text", t.textDetail1, true, false)
	t.pages.AddPage("form advanced", t.formAdvanced, true, false)
	// textMain1.Highlight("0")

	t.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.textTop, 5, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(t.listNav, 0, 1, true).
			AddItem(t.pages, 0, 4, false), 0, 5, false)
	t.flex.AddItem(t.textFooter, 3, 0, false)

	t.renderFooter(t.pbConfig.ConfigFilePath)
	t.renderHeader()

	t.setupKeyboard()

	// Start the application.
	if err := t.app.SetRoot(t.flex, true).SetFocus(t.listNav).Run(); err != nil {
		panic(err)
	}

	slog.Debug("Returning address to TUI")
	return &t, nil
}

// Start starts terminal user interface application.
func (tui *TUI) Start() error {
	err := tui.app.SetRoot(tui.flex, true).EnableMouse(true).Run()
	return err
}

// Stop quits the terminal user interface application.
func (tui *TUI) Stop() error {
	slog.Debug("Stopping TUI")
	tui.app.Stop()
	return nil
}
