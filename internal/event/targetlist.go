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

package event

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	// The maximum allowed number of concurrent Send() calls to all configured notifications targets
	maxConcurrentTargetSendCalls = 20000
)

// Target - event target interface
type Target interface {
	ID() TargetID
	IsActive() (bool, error)
	Save(Event) error
	Send(string) error
	Close() error
	HasQueueStore() bool
}

// TargetList - holds list of targets indexed by target ID.
type TargetList struct {
	// The number of concurrent async Send calls to all targets
	currentSendCalls int64

	sync.RWMutex
	targets map[TargetID]Target
}

// Add - adds unique target to target list.
func (list *TargetList) Add(targets ...Target) error {
	list.Lock()
	defer list.Unlock()

	for _, target := range targets {
		if _, ok := list.targets[target.ID()]; ok {
			return fmt.Errorf("target %v already exists", target.ID())
		}
		list.targets[target.ID()] = target
	}

	return nil
}

// Exists - checks whether target by target ID exists or not.
func (list *TargetList) Exists(id TargetID) bool {
	list.RLock()
	defer list.RUnlock()

	_, found := list.targets[id]
	return found
}

// TargetIDResult returns result of Remove/Send operation, sets err if
// any for the associated TargetID
type TargetIDResult struct {
	// ID where the remove or send were initiated.
	ID TargetID
	// Stores any error while removing a target or while sending an event.
	Err error
}

// Remove - closes and removes targets by given target IDs.
func (list *TargetList) Remove(targetIDSet TargetIDSet) {
	list.Lock()
	defer list.Unlock()

	for id := range targetIDSet {
		target, ok := list.targets[id]
		if ok {
			target.Close()
			delete(list.targets, id)
		}
	}
}

// Targets - list all targets
func (list *TargetList) Targets() []Target {
	if list == nil {
		return []Target{}
	}

	list.RLock()
	defer list.RUnlock()

	targets := []Target{}
	for _, tgt := range list.targets {
		targets = append(targets, tgt)
	}

	return targets
}

// List - returns available target IDs.
func (list *TargetList) List() []TargetID {
	list.RLock()
	defer list.RUnlock()

	keys := []TargetID{}
	for k := range list.targets {
		keys = append(keys, k)
	}

	return keys
}

// TargetMap - returns available targets.
func (list *TargetList) TargetMap() map[TargetID]Target {
	list.RLock()
	defer list.RUnlock()
	return list.targets
}

// Send - sends events to targets identified by target IDs.
func (list *TargetList) Send(event Event, targetIDset TargetIDSet, resCh chan<- TargetIDResult) {
	if atomic.LoadInt64(&list.currentSendCalls) > maxConcurrentTargetSendCalls {
		err := fmt.Errorf("concurrent target notifications exceeded %d", maxConcurrentTargetSendCalls)
		for id := range targetIDset {
			resCh <- TargetIDResult{ID: id, Err: err}
		}
		return
	}

	go func() {
		var wg sync.WaitGroup
		for id := range targetIDset {
			list.RLock()
			target, ok := list.targets[id]
			list.RUnlock()
			if ok {
				wg.Add(1)
				go func(id TargetID, target Target) {
					atomic.AddInt64(&list.currentSendCalls, 1)
					defer atomic.AddInt64(&list.currentSendCalls, -1)
					defer wg.Done()
					tgtRes := TargetIDResult{ID: id}
					if err := target.Save(event); err != nil {
						tgtRes.Err = err
					}
					resCh <- tgtRes
				}(id, target)
			} else {
				resCh <- TargetIDResult{ID: id}
			}
		}
		wg.Wait()
	}()
}

// NewTargetList - creates TargetList.
func NewTargetList() *TargetList {
	return &TargetList{targets: make(map[TargetID]Target)}
}
