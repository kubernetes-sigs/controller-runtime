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
	set        bool
	newEncoder zapcore.Encoder
	str        string
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
		v.newEncoder = newJSONEncoder()
		fmt.Printf("got JSON : %p \n", v.newEncoder)

	case "console":
		v.newEncoder = newConsoleEncoder()
		fmt.Printf("got CONSOLE : %p \n", v.newEncoder)

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
	set   bool
	level zap.AtomicLevel
}

func (v *levelValue) Set(l string) error {
	v.set = true
	lower := strings.ToLower(l)
	var lvl zap.AtomicLevel
	switch lower {
	case "debug":
		lvl = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		lvl = zap.NewAtomicLevelAt(zap.InfoLevel)
	default:
		return fmt.Errorf("invalid log level \"%s\"", l)
	}
	//v.level.SetLevel(zapcore.Level(int8(lvl)))
	v.level = lvl
	fmt.Println("*******", v.level)

	return nil
}

func (v levelValue) String() string {
	return v.level.String()
}

func (v levelValue) Type() string {
	return "level"
}

type stackTraceValue struct {
	set bool
	lv  zap.AtomicLevel
}

func (s *stackTraceValue) Set(val string) error {
	s.set = true
	lower := strings.ToLower(val)
	//var lv1 zap.AtomicLevel
	switch lower {
	case "warn":
		s.lv = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		s.lv = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		return fmt.Errorf("invalid stacktrace level \"%s\"", val)
	}
	//s.lv = lv1
	return nil
}

func (s stackTraceValue) String() string {
	return s.lv.String()
}

func (_ stackTraceValue) Type() string {
	return "lv"
}
