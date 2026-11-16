// Package utils holds small helpers shared across cutie's packages: running CLIs,
// formatting numbers and times, and laying out terminal text.
package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CmdError is a CLI that failed. It keeps both streams, because gh writes GraphQL
// errors to stdout and herdr writes JSON errors to stderr.
type CmdError struct {
	Cmd    string
	Stdout []byte
	Stderr []byte
	Err    error
}

func (e *CmdError) Error() string {
	if msg := strings.TrimSpace(string(e.Stderr)); msg != "" {
		return e.Cmd + ": " + msg
	}
	return e.Cmd + ": " + e.Err.Error()
}

func (e *CmdError) Unwrap() error { return e.Err }

// Run runs a command and returns its stdout, also when it fails. A failure is a
// *CmdError named after the command and its first argument, e.g. "herdr agent".
func Run(name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &CmdError{Cmd: label(name, args), Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Err: err}
	}
	return stdout.Bytes(), nil
}

// RunJSON runs a command and decodes its stdout as JSON into v. Leading non-JSON
// output (progress lines some CLIs print first) is skipped.
func RunJSON(v any, name string, args ...string) error {
	out, err := Run(name, args...)
	if err != nil {
		return err
	}
	if i := bytes.IndexAny(out, "{["); i > 0 {
		out = out[i:]
	}
	return Wrap(label(name, args)+": decoding", json.Unmarshal(out, v))
}

// AsCmdError returns the *CmdError inside err, if any.
func AsCmdError(err error) (*CmdError, bool) {
	var ce *CmdError
	ok := errors.As(err, &ce)
	return ce, ok
}

// Wrap prefixes err with what was being done, keeping nil as nil.
func Wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}

func label(name string, args []string) string {
	if len(args) > 0 {
		return name + " " + args[0]
	}
	return name
}
