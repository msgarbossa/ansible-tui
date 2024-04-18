package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
			Err: errors.New("relative paths not allowed"),
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
	err := os.Mkdir(dirName, os.ModeDir)
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

func writeFileFromString(outputFilePath string, contents string) error {
	b := []byte(contents) // convert input string (contents) to byte value
	err := os.WriteFile(outputFilePath, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", outputFilePath))
		return err
	}

	return nil
}
