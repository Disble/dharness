package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Writer records what a run touched so the whole thing can be undone.
//
// Every file change setup owns is snapshotted before the first byte changes. A
// file that did not exist is removed on undo; a file that did is put back
// exactly as it was, contents and mode. External side effects register their
// compensation alongside those snapshots.
type Writer struct {
	touched       []snapshot
	compensations []func() error
}

type snapshot struct {
	path    string
	existed bool
	data    []byte
	mode    os.FileMode
}

func (w *Writer) compensate(action func() error) {
	w.compensations = append(w.compensations, action)
}

// Write replaces a file, remembering what was there.
func (w *Writer) Write(path string, data []byte) error {
	if err := w.remember(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteJSON writes an indented JSON document with a trailing newline, which is
// what every tool here reads and what a diff stays readable in.
func (w *Writer) WriteJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return w.Write(path, append(raw, '\n'))
}

func (w *Writer) remember(path string) error {
	for _, taken := range w.touched {
		if taken.path == path {
			return nil
		}
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		w.touched = append(w.touched, snapshot{path: path})
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s before changing it: %w", path, err)
	}
	w.touched = append(w.touched, snapshot{path: path, existed: true, data: data, mode: info.Mode().Perm()})
	return nil
}

// Undo restores every file this run touched, in reverse order.
//
// Failures are collected rather than returned on the first one: a restore that
// stops halfway leaves a worse state than one that tries everything and then
// says exactly what it could not put back.
func (w *Writer) Undo() error {
	var failures error
	for i := len(w.compensations) - 1; i >= 0; i-- {
		failures = errors.Join(failures, w.compensations[i]())
	}

	for i := len(w.touched) - 1; i >= 0; i-- {
		taken := w.touched[i]

		if !taken.existed {
			if err := os.Remove(taken.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = errors.Join(failures, fmt.Errorf("remove %s: %w", taken.path, err))
			}
			continue
		}
		if err := os.WriteFile(taken.path, taken.data, taken.mode); err != nil {
			failures = errors.Join(failures, fmt.Errorf("restore %s: %w", taken.path, err))
		}
	}
	return failures
}
