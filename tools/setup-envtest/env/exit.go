/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package env

import (
	"errors"
	"fmt"
	"os"
)

// Exit exits with the given code and error message.
//
// Defer HandleExitWithCode in main to catch this and get the right behavior.
func Exit(code int, msg string, args ...any) {
	panic(&exitCode{
		code: code,
		err:  fmt.Errorf(msg, args...),
	})
}

// ExitCause exits with the given code and error message, automatically
// wrapping the underlying error passed as well.
//
// Defer HandleExitWithCode in main to catch this and get the right behavior.
func ExitCause(code int, err error, msg string, args ...any) {
	args = append(args, err)
	panic(&exitCode{
		code: code,
		err:  fmt.Errorf(msg+": %w", args...),
	})
}

// exitCode is an error that indicates, on a panic, to exit with the given code
// and message.
type exitCode struct {
	code int
	err  error
}

func (c *exitCode) Error() string {
	return fmt.Sprintf("%v (exit code %d)", c.err, c.code)
}
func (c *exitCode) Unwrap() error {
	return c.err
}

// asExit checks if the given (panic) value is an exitCode error,
// and if so stores it in the given pointer.  It's roughly analogous
// to errors.As, except it works on recover() values.
func asExit(val any, exit **exitCode) bool {
	if val == nil {
		return false
	}
	err, isErr := val.(error)
	if !isErr {
		return false
	}
	if !errors.As(err, exit) {
		return false
	}
	return true
}

// HandleExitWithCode handles panics of type exitCode,
// printing the status message and existing with the given
// exit code, or re-raising if not an exitCode error.
//
// This should be the first defer in your main function.
func HandleExitWithCode() {
	if cause := recover(); CheckRecover(cause, func(code int, err error) {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(code)
	}) {
		panic(cause)
	}
}

// CheckRecover checks the value of cause, calling the given callback
// if it's an exitCode error.  It returns true if we should re-panic
// the cause.
//
// It's mainly useful for testing, normally you'd use HandleExitWithCode.
func CheckRecover(cause any, cb func(int, error)) bool {
	if cause == nil {
		return false
	}
	var exitErr *exitCode
	if !asExit(cause, &exitErr) {
		// re-raise if it's not an exit error
		return true
	}

	cb(exitErr.code, exitErr.err)
	return false
}
