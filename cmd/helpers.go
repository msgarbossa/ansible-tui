package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// global variables
var (
	regExpDblDots  = regexp.MustCompile(`\.\.`)
	regExpPathName = regexp.MustCompile(`^[a-zA-Z0-9./\-_]+$`)
	regExpDotSlash = regexp.MustCompile(`^\./`)
)

// Check path is valid, no special characters, and no "..".
func sanitizePath(path string) error {

	if !regExpPathName.MatchString(path) {
		return &InputError{
			Err: errors.New("invalid characters in path"),
		}
	}

	m := regExpDblDots.FindString(path)
	if m != "" {
		return &InputError{
			Err: errors.New("relative back traversal not allowed in paths"),
		}
	}

	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if path != resolvedPath {
		slog.Debug(fmt.Sprintf("resolved path: %s", resolvedPath))
	}

	return nil
}

// Note: This function is very Unix-specific (not Windows compatible).
//
//	It also may have issues if dir = "/" or with symlinks.
func ListDir(dir string, exts []string) ([]string, error) {
	var files []string
	slog.Debug(fmt.Sprintf("Listing directory: %s", dir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files, err
	}
	for _, e := range entries {
		// fmt.Println(e.Name())
		path := e.Name()
		for _, s := range exts {
			if s == "" && !strings.Contains(path, ".") {
				path = dir + "/" + path
				files = append(files, path)
				break
			}
			if strings.HasSuffix(path, "."+s) {
				path = dir + "/" + path
				files = append(files, path)
				break
			}
		}
	}
	return files, nil
}

// Note: This function is very Unix-specific (not Windows compatible).
//
//	It also may have issues if dir = "/" or with symlinks.
func WalkDir(root string, exts []string) ([]string, error) {
	var files []string
	slog.Debug(fmt.Sprintf("Listing directory: %s", root))
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		for _, s := range exts {
			if s == "" && !strings.Contains(path, ".") {
				if !strings.HasPrefix(path, "/") {
					path = "./" + path
				}
				files = append(files, path)
				return nil
			}
			if strings.HasSuffix(path, "."+s) {
				if !strings.HasPrefix(path, "/") {
					path = "./" + path
				}
				files = append(files, path)
				return nil
			}
		}
		return nil
	})
	return files, err
}

func checkRelativePath(path string) bool {
	m := regExpDotSlash.FindString(path)
	if m != "" {
		return true
	} else {
		return false
	}
}

func checkAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	} else {
		return false
	}
}

func pathExists(path string, dir bool) (bool, error) {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, err
		// return false, &InputError{
		// 	Err: errors.New("path does not exist"),
		// }
	}
	if dir {
		if !stat.IsDir() {
			slog.Warn(fmt.Sprintf("Path was expected to be a directory, not a file: %s", path))
			return false, &InputError{
				Err: errors.New("path must be a directory"),
			}
		}
	} else {
		if stat.IsDir() {
			return false, &InputError{
				Err: errors.New("path must be a file, not a directory"),
			}
		}
	}
	return true, nil
}

func ensureDir(dirName string) error {
	err := os.Mkdir(dirName, 0750)
	if err == nil {
		return nil
	}
	if os.IsExist(err) {
		// check that the existing path is a directory
		info, err := os.Stat(dirName)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("path exists but is not a directory")
		}
		return nil
	}
	return err
}

func WriteFileFromString(outputFilePath string, contents string) error {
	b := []byte(contents) // convert input string (contents) to byte value
	err := os.WriteFile(outputFilePath, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", outputFilePath))
		return err
	}

	return nil
}

func GetContainerRuntime() (string, error) {

	_, err := exec.LookPath("podman")
	if err == nil {
		slog.Debug("found podman")
		return "podman", nil
	}

	_, err = exec.LookPath("docker")
	if err == nil {
		slog.Debug("found docker")
		return "docker", nil
	}

	return "", errors.New("no container runtime found")

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

func RunBufferedCommand(command string, cmdArgs []string, timeoutSeconds int, captureOutput bool, captureFilePath string) (int, *[]string, error) {

	var outputLines []string

	ctx, cancel := context.WithCancel(context.Background())
	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	}

	defer func() {
		for {
			select {
			case <-ctx.Done():
				// fmt.Println(ctx.Err())
				// slog.Error(fmt.Sprintf("command context: %s", ctx.Err()))
				return
			default:
				//fmt.Println("finished")
				// slog.Info("command context finished")
				cancel()
			}
		}
	}()

	cmd := exec.CommandContext(ctx, command, cmdArgs...)
	slog.Info(cmd.String())

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("could not get stderr pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("could not get stdout pipe: %v", err)
	}

	// defer cmd.Wait()

	go func() {
		// defer stdout.Close()
		var writer *bufio.Writer
		if captureFilePath != "" {
			f, err := os.Create(captureFilePath)
			if err != nil {
				slog.Error(fmt.Sprintf("Error creating capture file: %s", captureFilePath))

			}
			writer = bufio.NewWriter(f)
			defer writer.Flush()
			defer f.Close()

		}

		merged := io.MultiReader(stdout, stderr) // order of readers matters

		reader := bufio.NewReader(merged)
		for {
			strline, err := readLine(reader)
			if err != nil && err != io.EOF {
				slog.Warn(fmt.Sprintf("Reading line from buffered output: %s", err))
				break
				//log.Fatal(err)
			}

			if captureOutput {
				outputLines = append(outputLines, strline)
			}
			fmt.Println(strline)
			if captureFilePath != "" {
				writer.WriteString(strline + "\n")
			}

			if err == io.EOF {
				slog.Debug(fmt.Sprintf("EOF.  Done reading buffered output from command: %s", command))
				break
			}
		}
	}()

	slog.Info("Starting command execution")
	if err := cmd.Start(); err != nil {
		slog.Error(fmt.Sprintf("cmd.Start: %s", err))
		return 1, &outputLines, err
	}

	rc := 0
	pid := cmd.Process.Pid
	slog.Info(fmt.Sprintf("PID for command %s: %d", command, pid))

	if err = cmd.Wait(); err != nil {
		slog.Warn(fmt.Sprintf("Command wait error to get return code not nil: %s", err))
		if exiterr, ok := err.(*exec.ExitError); ok {
			slog.Info(fmt.Sprintf("Exit Status: %d", exiterr.ExitCode()))
			rc = exiterr.ExitCode()
		} else {
			slog.Error(fmt.Sprintf("cmd.Wait: %v", err))
			// rc = exiterr.ExitCode()
			rc = 1
		}
	}

	slog.Info(fmt.Sprintf("command finished: cmd=%s, rc=%d", command, rc))
	return rc, &outputLines, err
}

// TODO: Should make this pointer receiver method on PlaybookConfig struct (uses image, ssh private key, and temp dir path)
func executeCommandInContainer(c PlaybookConfig, command string, cmdArgs []string, captureOutput bool, captureFilePath string) (int, *[]string, error) {

	var outputLines []string

	slog.Debug("Starting executePlaybookInContainer()")

	// Determine container runtime
	containerRunCmd, err := GetContainerRuntime()
	if err != nil {
		slog.Error(fmt.Sprintf("%s", err))
		return 1, &outputLines, &ExecutionError{
			Err: errors.New("container image was specified, but no container runtime could be found (docker or podman)"),
		}
	}
	slog.Info(fmt.Sprintf("Using %s for container runtime", containerRunCmd))

	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("could not get current working directory")
		return 1, &outputLines, err
	}
	volMount1 := cwd + ":" + "/app:rw,z"

	// Convert relative path to temp container config file to path mounted inside the container
	containerConfigFile := c.TempDirPath + "/container-config.yml"
	containerConfigEnvVar := strings.Replace(containerConfigFile, "./", "", 1)
	containerConfigEnvVar = "PB_CONFIG_FILE=/app/" + containerConfigEnvVar

	// Set additional container runtime arguments
	var containerArgs []string
	containerArgs = append(containerArgs, "run", "--rm", "-u", "root", "-e", containerConfigEnvVar, "-e", "NO_TUI=true", "-v", volMount1)

	if c.SshPrivateKeyFile != "" {
		c.SshPrivateKeyFile, err = filepath.EvalSymlinks(c.SshPrivateKeyFile)
		if err != nil {
			return 1, &outputLines, err
		}
		// set volume mount to normalized location in container and append to command
		volMount2 := c.SshPrivateKeyFile + ":" + "/app/.ssh/ansible-tui:ro" // using mount option -z/-Z causes lsetxattr error
		containerArgs = append(containerArgs, "-v", volMount2)

		// modify SSH private key path to location used inside the container
		c.SshPrivateKeyFile = "/app/.ssh/ansible-tui"
	}

	containerArgs = append(containerArgs, c.Image, command)

	if len(cmdArgs) > 0 {
		containerArgs = append(containerArgs, cmdArgs...)
	}

	// unset image in PlaybookConfig before marshal for execution inside container
	c.Image = ""

	// write PlaybookConfig just before execution since values were modified accordingly above
	b, err := yaml.Marshal(&c)
	if err != nil {
		slog.Error("Could not marshal PlaybookConfig to bytes")
		return 1, &outputLines, err
	}
	err = os.WriteFile(containerConfigFile, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", containerConfigFile))
		return 1, &outputLines, err
	}

	// TODO: Should run a separate image pull here so container execution time is more predictable relative to supplied timout.
	// With separate pull can just add a few seconds.

	// Set container execution timeout to unlimited (-1) until above TODO for image pull is working
	return RunBufferedCommand(containerRunCmd, containerArgs, -1, captureOutput, captureFilePath)

}
