package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
)

// global variables
var (
	BuildVersion string = ""
	BuildDate    string = ""
	httpAddr     string = ""
)

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

func generateTemplateFile() {

	contents := `---
# virtual_env_path: "~/Documents/venv/ansible-latest
image: ansible-shim:latest
ssh_private_key_file: "~/.ssh/id_rsa"
remote_user: root
inventory: "./examples/hosts.yml"
playbook: "./examples/site.yml"
verbose_level: 1`

	writeFileFromString("./ansible-shim.yml", contents)
}

func main() {

	// default log level to LevelWarn (prints WARN and ERROR)
	slog.SetLogLoggerLevel(slog.LevelWarn)

	// To allow for testing, os.Exit() should only be done from this main() function.
	// All other functions should return errors (nil when no errors).

	// read and parse CLI options
	pbConfigFile := ""
	displayVersion := flag.Bool("version", false, "Display version and exit")
	generateTemplate := flag.Bool("g", false, "Generate ansible-shim.yml template and exit")
	flag.StringVar(&httpAddr, "httpAddr", LookupEnvOrString("HTTP_ADDR", ""), "ip:port to listen for http requests, env=HTTP_ADDR")
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

	if *generateTemplate {
		generateTemplateFile()
		os.Exit(0)
	}

	// set log level accoring to -v / -vv CLI options
	if *logLevel1 {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}
	if *logLevel2 {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if httpAddr != "" {
		mainListener()
		os.Exit(0)
	}

	rc, err := handlePlaybookParams(pbConfigFile)

	// Final exit code is based on the results of above runAnsiblePlaybook method call
	if err != nil {
		os.Exit(rc)
	}
	os.Exit(rc)

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
