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
	"flag"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog"
)

type encoderConfigFunc func(*zapcore.EncoderConfig)

type encoderValue struct {
	set        bool
	newEncoder zapcore.Encoder
	str        string
}

func (v *encoderValue) Set(e string) error {
	v.set = true
	if e == "json" || e == "console" {
		v.newEncoder = newEncoder(e)
	} else {
		return fmt.Errorf("unknown encoder \"%s\"", e)
	}
	v.str = e
	return nil
}

func (v encoderValue) String() string {
	return v.str
}

func (v encoderValue) Type() string {
	return "encoder"
}

func newEncoder(e string, ecfs ...encoderConfigFunc) zapcore.Encoder {
	if e == "json" {
		encoderConfig := zap.NewProductionEncoderConfig()
		for _, f := range ecfs {
			f(&encoderConfig)
		}
		fmt.Println("HERE WITH JSON")
		return zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig := zap.NewDevelopmentEncoderConfig()
		for _, f := range ecfs {
			f(&encoderConfig)
		}
		fmt.Println("HERE WITH CONSOLE")
		return zapcore.NewConsoleEncoder(encoderConfig)
	}

}

type levelValue struct {
	set   bool
	level zap.AtomicLevel
}

func (v *levelValue) Set(l string) error {
	v.set = true
	lower := strings.ToLower(l)
	var lvl int
	switch lower {
	case "debug":
		lvl = -1
	case "info":
		lvl = 0
	case "error":
		lvl = 2
	default:
		i, err := strconv.Atoi(lower)
		if err != nil {
			return fmt.Errorf("invalid log level \"%s\"", l)
		}

		if i > 0 {
			lvl = -1 * i
		} else {
			return fmt.Errorf("NO LIKEY log level \"%s\"", l)
		}
	}
	v.level.SetLevel(zapcore.Level(int8(lvl)))
	// If log level is greater than debug, set glog/klog level to that level.
	if lvl < -3 {
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		klog.InitFlags(fs)
		err := fs.Set("v", fmt.Sprintf("%v", -1*lvl))
		if err != nil {
			return err
		}
	}
	return nil
}

func (v levelValue) String() string {
	return v.level.String()
}

func (v levelValue) Type() string {
	return "level"
}
