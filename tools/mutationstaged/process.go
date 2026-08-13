package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type commandSpec struct {
	Dir    string
	Name   string
	Args   []string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

type processRunner interface {
	Output(dir, name string, args ...string) ([]byte, error)
	Run(commandSpec) error
}

// gitContextVariables point Git at a specific repository regardless of where a
// process is running.
//
// Git exports them to hooks, and in a linked worktree it exports them as
// absolute paths. Anything spawned from a hook then inherits them, so a command
// meant for the directory it was handed addresses the hook's repository
// instead. Measured twice: once leaving a stray commit on a live branch, and
// once writing core.bare and a user identity into a shared config, which
// misattributed every commit made afterwards.
//
// This wrapper resolves its own root from the directory it was pointed at, so
// an ambient repository is never what it means. Stripping these is how that
// intent survives being run from a hook.
var gitContextVariables = []string{
	"GIT_DIR=",
	"GIT_INDEX_FILE=",
	"GIT_WORK_TREE=",
	"GIT_OBJECT_DIRECTORY=",
	"GIT_COMMON_DIR=",
}

// environmentWithoutGitContext is os.Environ minus the variables above.
func environmentWithoutGitContext() []string {
	return stripGitContext(os.Environ())
}

func stripGitContext(environment []string) []string {
	stripped := make([]string, 0, len(environment))
	for _, entry := range environment {
		inherited := false
		for _, prefix := range gitContextVariables {
			if strings.HasPrefix(entry, prefix) {
				inherited = true
				break
			}
		}
		if !inherited {
			stripped = append(stripped, entry)
		}
	}
	return stripped
}

type osProcessRunner struct{}

func (osProcessRunner) Output(dir, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = environmentWithoutGitContext()
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %v: %w: %s", name, args, err, bytes.TrimSpace(output))
	}
	return output, nil
}

func (osProcessRunner) Run(spec commandSpec) error {
	command := exec.Command(spec.Name, spec.Args...)
	command.Dir = spec.Dir
	command.Env = spec.Env
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	return command.Run()
}
