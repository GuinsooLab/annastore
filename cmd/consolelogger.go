// Copyright (c) 2022 GuinsooLab
//
// This file is part of GuinsooLab stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"container/ring"
	"context"
	"sync"

	"github.com/GuinsooLab/annastore/internal/logger"
	"github.com/GuinsooLab/annastore/internal/logger/message/log"
	"github.com/GuinsooLab/annastore/internal/logger/target/console"
	"github.com/GuinsooLab/annastore/internal/logger/target/types"
	"github.com/GuinsooLab/annastore/internal/pubsub"
	"github.com/minio/madmin-go"
	xnet "github.com/minio/pkg/net"
)

// number of log messages to buffer
const defaultLogBufferCount = 10000

// HTTPConsoleLoggerSys holds global console logger state
type HTTPConsoleLoggerSys struct {
	sync.RWMutex
	pubsub   *pubsub.PubSub
	console  *console.Target
	nodeName string
	logBuf   *ring.Ring
}

// NewConsoleLogger - creates new HTTPConsoleLoggerSys with all nodes subscribed to
// the console logging pub sub system
func NewConsoleLogger(ctx context.Context) *HTTPConsoleLoggerSys {
	ps := pubsub.New(8)
	return &HTTPConsoleLoggerSys{
		pubsub:  ps,
		console: console.New(),
		logBuf:  ring.New(defaultLogBufferCount),
	}
}

// SetNodeName - sets the node name if any after distributed setup has initialized
func (sys *HTTPConsoleLoggerSys) SetNodeName(nodeName string) {
	if !globalIsDistErasure {
		sys.nodeName = ""
		return
	}

	host, err := xnet.ParseHost(globalLocalNodeName)
	if err != nil {
		logger.FatalIf(err, "Unable to start console logging subsystem")
	}

	sys.nodeName = host.Name
}

// HasLogListeners returns true if console log listeners are registered
// for this node or peers
func (sys *HTTPConsoleLoggerSys) HasLogListeners() bool {
	return sys != nil && sys.pubsub.NumSubscribers(madmin.LogMaskAll) > 0
}

// Subscribe starts console logging for this node.
func (sys *HTTPConsoleLoggerSys) Subscribe(subCh chan pubsub.Maskable, doneCh <-chan struct{}, node string, last int, logKind madmin.LogMask, filter func(entry pubsub.Maskable) bool) error {
	// Enable console logging for remote client.
	if !sys.HasLogListeners() {
		logger.AddSystemTarget(sys)
	}

	cnt := 0
	// by default send all console logs in the ring buffer unless node or limit query parameters
	// are set.
	var lastN []log.Info
	if last > defaultLogBufferCount || last <= 0 {
		last = defaultLogBufferCount
	}

	lastN = make([]log.Info, last)
	sys.RLock()
	sys.logBuf.Do(func(p interface{}) {
		if p != nil {
			lg, ok := p.(log.Info)
			if ok && lg.SendLog(node, logKind) {
				lastN[cnt%last] = lg
				cnt++
			}
		}
	})
	sys.RUnlock()
	// send last n console log messages in order filtered by node
	if cnt > 0 {
		for i := 0; i < last; i++ {
			entry := lastN[(cnt+i)%last]
			if (entry == log.Info{}) {
				continue
			}
			select {
			case subCh <- entry:
			case <-doneCh:
				return nil
			}
		}
	}
	return sys.pubsub.Subscribe(pubsub.MaskFromMaskable(madmin.LogMaskAll), subCh, doneCh, filter)
}

// Init if HTTPConsoleLoggerSys is valid, always returns nil right now
func (sys *HTTPConsoleLoggerSys) Init() error {
	return nil
}

// Endpoint - dummy function for interface compatibility
func (sys *HTTPConsoleLoggerSys) Endpoint() string {
	return sys.console.Endpoint()
}

// String - stringer function for interface compatibility
func (sys *HTTPConsoleLoggerSys) String() string {
	return logger.ConsoleLoggerTgt
}

// Content returns the console stdout log
func (sys *HTTPConsoleLoggerSys) Content() (logs []log.Entry) {
	sys.RLock()
	sys.logBuf.Do(func(p interface{}) {
		if p != nil {
			lg, ok := p.(log.Info)
			if ok {
				if (lg.Entry != log.Entry{}) {
					logs = append(logs, lg.Entry)
				}
			}
		}
	})
	sys.RUnlock()

	return
}

// Cancel - cancels the target
func (sys *HTTPConsoleLoggerSys) Cancel() {
}

// Type - returns type of the target
func (sys *HTTPConsoleLoggerSys) Type() types.TargetType {
	return types.TargetConsole
}

// Send log message 'e' to console and publish to console
// log pubsub system
func (sys *HTTPConsoleLoggerSys) Send(entry interface{}) error {
	var lg log.Info
	switch e := entry.(type) {
	case log.Entry:
		lg = log.Info{Entry: e, NodeName: sys.nodeName}
	case string:
		lg = log.Info{ConsoleMsg: e, NodeName: sys.nodeName}
	}

	sys.pubsub.Publish(lg)
	sys.Lock()
	// add log to ring buffer
	sys.logBuf.Value = lg
	sys.logBuf = sys.logBuf.Next()
	sys.Unlock()

	return sys.console.Send(entry, string(logger.All))
}
