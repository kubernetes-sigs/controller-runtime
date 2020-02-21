/*
Copyright 2019 The Kubernetes Authors.

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

// Package zap contains helpers for setting up a new logr.Logger instance
// using the Zap logging framework.

package zap

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type encoderConfigFunc func(*zapcore.EncoderConfig)

type encoderFlag struct {
	setFunc func(zapcore.Encoder)
	value   string
}

var _ pflag.Value = &encoderFlag{}

func (ev *encoderFlag) String() string {
	return ev.value
}

func (ev *encoderFlag) Type() string {
	return "encoder"
}

func (ev *encoderFlag) Set(flagValue string) error {
	val := strings.ToLower(flagValue)
	switch val {
	case "json":
		ev.setFunc(newJSONEncoder())
	case "console":
		ev.setFunc(newConsoleEncoder())
	default:
		return fmt.Errorf("invalid encoder value \"%s\"", flagValue)
	}
	ev.value = flagValue
	return nil
}

func newJSONEncoder(ecfs ...encoderConfigFunc) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	for _, f := range ecfs {
		f(&encoderConfig)
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

func newConsoleEncoder(ecfs ...encoderConfigFunc) zapcore.Encoder {
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	for _, f := range ecfs {
		f(&encoderConfig)
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}

type levelFlag struct {
	setFunc func(zap.AtomicLevel)
	value   string
}

var _ pflag.Value = &levelFlag{}

func (ev *levelFlag) Set(flagValue string) error {
	lower := strings.ToLower(flagValue)
	switch lower {
	case "debug":
		ev.setFunc(zap.NewAtomicLevelAt(zap.DebugLevel))
	case "info":
		ev.setFunc(zap.NewAtomicLevelAt(zap.InfoLevel))
	default:
		return fmt.Errorf("invalid log level \"%s\"", flagValue)
	}
	ev.value = flagValue
	return nil
}

func (ev *levelFlag) String() string {
	return ev.value
}

func (ev *levelFlag) Type() string {
	return "level"
}

type stackTraceFlag struct {
	setFunc func(zap.AtomicLevel)
	value   string
}

func (ev *stackTraceFlag) Set(flagValue string) error {
	lower := strings.ToLower(flagValue)
	switch lower {
	case "warn":
		ev.setFunc(zap.NewAtomicLevelAt(zap.WarnLevel))
	case "error":
		ev.setFunc(zap.NewAtomicLevelAt(zap.ErrorLevel))
	default:
		return fmt.Errorf("invalid stacktrace level \"%s\"", flagValue)
	}
	ev.value = flagValue
	return nil
}

func (ev *stackTraceFlag) String() string {
	return ev.value
}

func (ev *stackTraceFlag) Type() string {
	return "level"
}
