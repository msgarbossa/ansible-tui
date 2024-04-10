package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
)

// global variables
var (
	regExpDblDots  = regexp.MustCompile(`\.\.`)
	regExpPathName = regexp.MustCompile(`^[a-zA-Z0-9./\-_]+$`)
)

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

func writeFileFromString(outputFilePath string, contents string) error {
	b := []byte(contents) // convert input string (contents) to byte value
	err := os.WriteFile(outputFilePath, b, 0600)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing output file: %s", outputFilePath))
		return err
	}

	return nil
}
