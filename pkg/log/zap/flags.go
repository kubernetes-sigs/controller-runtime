// Copyright 2019 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zap

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type encoderConfigFunc func(*zapcore.EncoderConfig)

type encoderValue struct {
	set     bool
	setFunc func(zapcore.Encoder)
	str     string
}

func (v encoderValue) String() string {
	return v.str
}

func (v encoderValue) Type() string {
	return "encoder"
}

func (v *encoderValue) Set(e string) error {
	v.set = true
	val := strings.ToLower(e)
	switch val {
	case "json":
		v.setFunc(newJSONEncoder())
	case "console":
		v.setFunc(newConsoleEncoder())
	default:
		return fmt.Errorf("invalid encoder value \"%s\"", e)
	}
	v.str = e
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

type levelValue struct {
	set     bool
	setFunc func(zap.AtomicLevel)
	str     string
}

func (v *levelValue) Set(l string) error {
	v.set = true
	lower := strings.ToLower(l)
	switch lower {
	case "debug":
		v.setFunc(zap.NewAtomicLevelAt(zap.DebugLevel))
	case "info":
		v.setFunc(zap.NewAtomicLevelAt(zap.InfoLevel))
	default:
		return fmt.Errorf("invalid log level \"%s\"", l)
	}
	v.str = l
	return nil
}

func (v levelValue) String() string {
	return v.str
}

func (v levelValue) Type() string {
	return "level"
}

type stackTraceValue struct {
	set     bool
	setFunc func(zap.AtomicLevel)
	str     string
}

func (s *stackTraceValue) Set(val string) error {
	s.set = true
	lower := strings.ToLower(val)
	switch lower {
	case "warn":
		s.setFunc(zap.NewAtomicLevelAt(zap.WarnLevel))
	case "error":
		s.setFunc(zap.NewAtomicLevelAt(zap.ErrorLevel))
	default:
		return fmt.Errorf("invalid stacktrace level \"%s\"", val)
	}
	return nil
}

func (s stackTraceValue) String() string {
	return s.str
}

func (_ stackTraceValue) Type() string {
	return "level"
}
