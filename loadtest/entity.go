// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package loadtest

import (
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mattermost/mattermost-load-test/autocreation"
	"github.com/mattermost/mattermost-load-test/cmdlog"
	"github.com/mattermost/mattermost-load-test/randutil"
	"github.com/mattermost/platform/model"
)

type EntityConfig struct {
	EntityNumber        int
	EntityName          string
	EntityActions       []randutil.Choice
	UserData            autocreation.UserImportData
	ChannelMap          map[string]string
	TeamMap             map[string]string
	Client              *model.Client4
	Client3             *model.Client
	WebSocketClient     *model.WebSocketClient
	ActionRate          time.Duration
	LoadTestConfig      *LoadTestConfig
	StatusReportChannel chan<- UserEntityStatusReport
	StopChannel         <-chan bool
	StopWaitGroup       *sync.WaitGroup
	Info                map[string]interface{}
}

func runEntity(ec *EntityConfig) {
	defer func() {
		if r := recover(); r != nil {
			cmdlog.Errorf("%s: %s", r, debug.Stack())
			ec.StopWaitGroup.Add(1)
			go runEntity(ec)
		}
	}()
	defer ec.StopWaitGroup.Done()
	time.Sleep(time.Millisecond * time.Duration(ec.EntityNumber*(ec.LoadTestConfig.UserEntitiesConfiguration.ActionRateMilliseconds*ec.LoadTestConfig.UserEntitiesConfiguration.NumActiveEntities)))
	timer := time.NewTimer(0)

	for {
		select {
		case <-ec.StopChannel:
			return
		case <-timer.C:
			action, err := randutil.WeightedChoice(ec.EntityActions)
			if err != nil {
				cmdlog.Error("Failed to pick weighted choice")
				return
			}
			action.Item.(func(*EntityConfig))(ec)
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(ec.LoadTestConfig.UserEntitiesConfiguration.ActionRateMaxVarianceMilliseconds)))
			timer.Reset(ec.ActionRate)
		}
	}
}

func doStatusPolling(ec *EntityConfig) {
	defer func() {
		if r := recover(); r != nil {
			cmdlog.Errorf("%s: %s", r, debug.Stack())
			ec.StopWaitGroup.Add(1)
			go doStatusPolling(ec)
		}
	}()
	defer ec.StopWaitGroup.Done()
	time.Sleep(time.Millisecond * time.Duration(ec.EntityNumber*(ec.LoadTestConfig.UserEntitiesConfiguration.ActionRateMilliseconds*ec.LoadTestConfig.UserEntitiesConfiguration.NumActiveEntities)))
	ticker := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-ec.StopChannel:
			return
		case <-ticker.C:
			actionGetStatuses(ec)
		}
	}
}

func websocketListen(ec *EntityConfig) {
	defer ec.StopWaitGroup.Done()

	if ec.WebSocketClient == nil {
		return
	}

	ec.WebSocketClient.Listen()

	websocketRetryCount := 0

	for {
		select {
		case <-ec.StopChannel:
			return
		case _, ok := <-ec.WebSocketClient.EventChannel:
			if !ok {
				// If we are set to retry connection, first retry immediately, then backoff until retry max is reached
				for {
					if websocketRetryCount > 5 {
						if ec.WebSocketClient.ListenError != nil {
							cmdlog.Errorf("Websocket Error: %v", ec.WebSocketClient.ListenError.Error())
						} else {
							cmdlog.Error("Server closed websocket")
						}
						cmdlog.Error("Websocket disconneced. Max retries reached.")
						return
					}
					time.Sleep(time.Duration(websocketRetryCount) * time.Second)
					if err := ec.WebSocketClient.Connect(); err != nil {
						websocketRetryCount++
						continue
					}
					ec.WebSocketClient.Listen()
					break
				}
			}
		}
	}
}

func (config *EntityConfig) SendStatus(status int, err error, details string) {
	config.StatusReportChannel <- UserEntityStatusReport{
		Status:  status,
		Err:     err,
		Config:  config,
		Details: details,
	}
}

func (config *EntityConfig) SendStatusLaunching() {
	config.SendStatus(STATUS_LAUNCHING, nil, "")
}

func (config *EntityConfig) SendStatusActive(details string) {
	config.SendStatus(STATUS_ACTIVE, nil, details)
}

func (config *EntityConfig) SendStatusError(err error, details string) {
	config.SendStatus(STATUS_ERROR, err, details)
}

func (config *EntityConfig) SendStatusFailedLaunch(err error, details string) {
	config.SendStatus(STATUS_FAILED_LAUNCH, err, details)
}

func (config *EntityConfig) SendStatusFailedActive(err error, details string) {
	config.SendStatus(STATUS_FAILED_ACTIVE, err, details)
}

func (config *EntityConfig) SendStatusActionSend(details string) {
	config.SendStatus(STATUS_ACTION_SEND, nil, details)
}

func (config *EntityConfig) SendStatusActionRecieve(details string) {
	config.SendStatus(STATUS_ACTION_RECIEVE, nil, details)
}

func (config *EntityConfig) SendStatusStopped(details string) {
	config.SendStatus(STATUS_STOPPED, nil, details)
}
