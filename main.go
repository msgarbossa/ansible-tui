package main

import (
	"a5e/cmd"
	"a5e/tui"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
)

// global variables
var (
	BuildVersion          string = ""
	BuildDate             string = ""
	defaultTempPath       string = "./.ansible-tui"
	defaultConfigFilePath string = "./.ansible-tui/config.yml"
)

func main() {

	// default log level to LevelWarn (prints WARN and ERROR)
	slog.SetLogLoggerLevel(slog.LevelWarn)

	// To allow for testing, os.Exit() should only be done from this main() function.
	// All other functions should return errors (nil when no errors).

	// read and parse CLI options
	pbConfigFile := ""
	displayVersion := flag.Bool("version", false, "Display version and exit")
	generateTemplate := flag.Bool("g", false, "Generate ansible-tui.yml template and exit")
	noTui := flag.Bool("nt", LookupEnvOrBool("NO_TUI", false), "No TUI.  Runs playbook from configuration file without TUI")
	flag.StringVar(&pbConfigFile, "c", LookupEnvOrString("PB_CONFIG_FILE", ""), "Playbook config file (PB_CONFIG_FILE)")
	logLevel1 := flag.Bool("v", false, "Sets log level for ansible-tui to INFO (default WARN)")
	logLevel2 := flag.Bool("vv", false, "Sets log level for ansible-tui to DEBUG (default WARN)")
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

	c := cmd.NewPlaybookConfig()

	// handle config file path and temp directory path
	if pbConfigFile != "" {
		c.ConfigFilePath = pbConfigFile
	} else {
		c.ConfigFilePath = defaultConfigFilePath
	}
	c.TempDirPath = defaultTempPath

	var err error

	if *generateTemplate {
		err = c.GenerateTemplateFile(defaultConfigFilePath)
		if err != nil {
			slog.Error(fmt.Sprintf("Exiting due to error generating initial configuration file: %s", err))
			os.Exit(1)
		}
		os.Exit(0)
	}

	// if no config file was passed, set force TUI (default config file will be used)
	if pbConfigFile == "" {
		*noTui = false
		pbConfigFile = defaultConfigFilePath
		if _, err := os.Stat(pbConfigFile); os.IsNotExist(err) {
			err = c.GenerateTemplateFile(defaultConfigFilePath)
			if err != nil {
				slog.Error(fmt.Sprintf("Exiting due to error generating initial configuration file: %s", err))
				os.Exit(1)
			}
		}
	}

	// read config file into PlaybookConfig struct
	err = c.ReadConf(pbConfigFile)
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due to config file error: %s", err))
		os.Exit(1)
	}

	// read environment variables into PlaybookConfig struct
	err = c.ReadEnvs()
	if err != nil {
		slog.Error(fmt.Sprintf("Exiting due error reading environment variables: %s", err))
		os.Exit(1)
	}

	// Start TUI if config file was not specified or if -nt flag was not passed
	if !*noTui {
		slog.Debug("Creating TUI")
		tui.BuildDate = BuildDate
		tui.BuildVersion = BuildVersion
		a, err := tui.NewTUI(c)
		if err != nil {
			slog.Error(fmt.Sprintf("Exiting due to errors starting TUI: %s", err))
			os.Exit(1)
		}

		slog.Debug("Starting TUI")
		err = a.Start()
		if err != nil {
			slog.Error(fmt.Sprintf("Exiting due to errors starting TUI: %s", err))
			os.Exit(1)
		}
		// This should exit within the TUI to ensure proper exit (below should never run)
		a.Stop()
		os.Exit(0)
	}

	// process values in PlaybookConfig struct
	err = c.ProcessEnvs()
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
