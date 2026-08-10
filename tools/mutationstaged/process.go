package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
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

type osProcessRunner struct{}

func (osProcessRunner) Output(dir, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
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
