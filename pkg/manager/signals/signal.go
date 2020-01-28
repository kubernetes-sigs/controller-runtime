/*
Copyright 2017 The Kubernetes Authors.

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

package signals

import (
	"os"
	"os/signal"
	"strconv"
	"time"
)

const (
	channelClosureDelaySecondsEnv = "CHANNEL_CLOSURE_DELAY_SECONDS"

	defaultChannelClosureDelaySeconds = 0
)

var onlyOneSignalHandler = make(chan struct{})

// SetupSignalHandler registers for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
func SetupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		d := getDelaySecondsFromEnv()
		log.Info("receive signal", "delay", d)
		time.Sleep(time.Duration(d) * time.Second)
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}

func getDelaySecondsFromEnv() int {
	delayStr := os.Getenv(channelClosureDelaySecondsEnv)
	if len(delayStr) == 0 {
		return defaultChannelClosureDelaySeconds
	}

	delay, err := strconv.Atoi(delayStr)
	if err != nil {
		log.Info("failed to convert channel closure delay", "envvar", delayStr)
		return defaultChannelClosureDelaySeconds
	}
	if delay < 0 {
		log.Info("get invalid channel closure delay from envvar", "envvar", delayStr)
		return 0
	}
	return delay
}
