// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	bucketBandwidth "github.com/GuinsooLab/annastore/internal/bucket/bandwidth"
	"github.com/GuinsooLab/annastore/internal/crypto"
	"github.com/GuinsooLab/annastore/internal/event"
	xhttp "github.com/GuinsooLab/annastore/internal/http"
	"github.com/GuinsooLab/annastore/internal/logger"
	"github.com/GuinsooLab/annastore/internal/sync/errgroup"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/cespare/xxhash/v2"
	"github.com/klauspost/compress/zip"
	"github.com/minio/madmin-go"
	"github.com/minio/pkg/bucket/policy"
	xnet "github.com/minio/pkg/net"
)

// NotificationSys - notification system.
type NotificationSys struct {
	sync.RWMutex
	targetList                 *event.TargetList
	targetResCh                chan event.TargetIDResult
	bucketRulesMap             map[string]event.RulesMap
	bucketRemoteTargetRulesMap map[string]map[event.TargetID]event.RulesMap
	peerClients                []*peerRESTClient // Excludes self
	allPeerClients             []*peerRESTClient // Includes nil client for self
}

// GetARNList - returns available ARNs.
func (sys *NotificationSys) GetARNList(onlyActive bool) []string {
	arns := []string{}
	if sys == nil {
		return arns
	}
	region := globalSite.Region
	for targetID, target := range sys.targetList.TargetMap() {
		// httpclient target is part of ListenNotification
		// which doesn't need to be listed as part of the ARN list
		// This list is only meant for external targets, filter
		// this out pro-actively.
		if !strings.HasPrefix(targetID.ID, "httpclient+") {
			if onlyActive && !target.HasQueueStore() {
				if _, err := target.IsActive(); err != nil {
					continue
				}
			}
			arns = append(arns, targetID.ToARN(region).String())
		}
	}

	return arns
}

// NotificationPeerErr returns error associated for a remote peer.
type NotificationPeerErr struct {
	Host xnet.Host // Remote host on which the rpc call was initiated
	Err  error     // Error returned by the remote peer for an rpc call
}

// A NotificationGroup is a collection of goroutines working on subtasks that are part of
// the same overall task.
//
// A zero NotificationGroup is valid and does not cancel on error.
type NotificationGroup struct {
	wg   sync.WaitGroup
	errs []NotificationPeerErr
}

// WithNPeers returns a new NotificationGroup with length of errs slice upto nerrs,
// upon Wait() errors are returned collected from all tasks.
func WithNPeers(nerrs int) *NotificationGroup {
	return &NotificationGroup{errs: make([]NotificationPeerErr, nerrs)}
}

// Wait blocks until all function calls from the Go method have returned, then
// returns the slice of errors from all function calls.
func (g *NotificationGroup) Wait() []NotificationPeerErr {
	g.wg.Wait()
	return g.errs
}

// Go calls the given function in a new goroutine.
//
// The first call to return a non-nil error will be
// collected in errs slice and returned by Wait().
func (g *NotificationGroup) Go(ctx context.Context, f func() error, index int, addr xnet.Host) {
	g.wg.Add(1)

	go func() {
		defer g.wg.Done()
		g.errs[index] = NotificationPeerErr{
			Host: addr,
		}
		for i := 0; i < 3; i++ {
			if err := f(); err != nil {
				g.errs[index].Err = err
				// Last iteration log the error.
				if i == 2 {
					reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", addr.String())
					ctx := logger.SetReqInfo(ctx, reqInfo)
					logger.LogIf(ctx, err)
				}
				// Wait for one second and no need wait after last attempt.
				if i < 2 {
					time.Sleep(1 * time.Second)
				}
				continue
			}
			break
		}
	}()
}

// DeletePolicy - deletes policy across all peers.
func (sys *NotificationSys) DeletePolicy(policyName string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.DeletePolicy(policyName)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// LoadPolicy - reloads a specific modified policy across all peers
func (sys *NotificationSys) LoadPolicy(policyName string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.LoadPolicy(policyName)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// LoadPolicyMapping - reloads a policy mapping across all peers
func (sys *NotificationSys) LoadPolicyMapping(userOrGroup string, isGroup bool) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.LoadPolicyMapping(userOrGroup, isGroup)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// DeleteUser - deletes a specific user across all peers
func (sys *NotificationSys) DeleteUser(accessKey string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.DeleteUser(accessKey)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// LoadUser - reloads a specific user across all peers
func (sys *NotificationSys) LoadUser(accessKey string, temp bool) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.LoadUser(accessKey, temp)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// LoadGroup - loads a specific group on all peers.
func (sys *NotificationSys) LoadGroup(group string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error { return client.LoadGroup(group) }, idx, *client.host)
	}
	return ng.Wait()
}

// DeleteServiceAccount - deletes a specific service account across all peers
func (sys *NotificationSys) DeleteServiceAccount(accessKey string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.DeleteServiceAccount(accessKey)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// LoadServiceAccount - reloads a specific service account across all peers
func (sys *NotificationSys) LoadServiceAccount(accessKey string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.LoadServiceAccount(accessKey)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// BackgroundHealStatus - returns background heal status of all peers
func (sys *NotificationSys) BackgroundHealStatus() ([]madmin.BgHealState, []NotificationPeerErr) {
	ng := WithNPeers(len(sys.peerClients))
	states := make([]madmin.BgHealState, len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		idx := idx
		client := client
		ng.Go(GlobalContext, func() error {
			st, err := client.BackgroundHealStatus()
			if err != nil {
				return err
			}
			states[idx] = st
			return nil
		}, idx, *client.host)
	}

	return states, ng.Wait()
}

// StartProfiling - start profiling on remote peers, by initiating a remote RPC.
func (sys *NotificationSys) StartProfiling(profiler string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.StartProfiling(profiler)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// DownloadProfilingData - download profiling data from all remote peers.
func (sys *NotificationSys) DownloadProfilingData(ctx context.Context, writer io.Writer) (profilingDataFound bool) {
	// Initialize a zip writer which will provide a zipped content
	// of profiling data of all nodes
	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	for _, client := range sys.peerClients {
		if client == nil {
			continue
		}
		data, err := client.DownloadProfileData()
		if err != nil {
			reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", client.host.String())
			ctx := logger.SetReqInfo(ctx, reqInfo)
			logger.LogIf(ctx, err)
			continue
		}

		profilingDataFound = true

		for typ, data := range data {
			err := embedFileInZip(zipWriter, fmt.Sprintf("profile-%s-%s", client.host.String(), typ), data)
			if err != nil {
				reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", client.host.String())
				ctx := logger.SetReqInfo(ctx, reqInfo)
				logger.LogIf(ctx, err)
			}
		}
	}

	// Local host
	thisAddr, err := xnet.ParseHost(globalLocalNodeName)
	if err != nil {
		logger.LogIf(ctx, err)
		return profilingDataFound
	}

	data, err := getProfileData()
	if err != nil {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", thisAddr.String())
		ctx := logger.SetReqInfo(ctx, reqInfo)
		logger.LogIf(ctx, err)
		return profilingDataFound
	}

	profilingDataFound = true

	// Send profiling data to zip as file
	for typ, data := range data {
		err := embedFileInZip(zipWriter, fmt.Sprintf("profile-%s-%s", thisAddr, typ), data)
		if err != nil {
			logger.LogIf(ctx, err)
		}
	}

	appendClusterMetaInfoToZip(ctx, zipWriter)
	return
}

// VerifyBinary - asks remote peers to verify the checksum
func (sys *NotificationSys) VerifyBinary(ctx context.Context, u *url.URL, sha256Sum []byte, releaseInfo string, reader []byte) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.VerifyBinary(ctx, u, sha256Sum, releaseInfo, reader)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// CommitBinary - asks remote peers to overwrite the old binary with the new one
func (sys *NotificationSys) CommitBinary(ctx context.Context) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.CommitBinary(ctx)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// SignalConfigReload reloads requested sub-system on a remote peer dynamically.
func (sys *NotificationSys) SignalConfigReload(subSys string) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.SignalService(serviceReloadDynamic, subSys)
		}, idx, *client.host)
	}
	return ng.Wait()
}

// SignalService - calls signal service RPC call on all peers.
func (sys *NotificationSys) SignalService(sig serviceSignal) []NotificationPeerErr {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.SignalService(sig, "")
		}, idx, *client.host)
	}
	return ng.Wait()
}

// updateBloomFilter will cycle all servers to the current index and
// return a merged bloom filter if a complete one can be retrieved.
func (sys *NotificationSys) updateBloomFilter(ctx context.Context, current uint64) (*bloomFilter, error) {
	req := bloomFilterRequest{
		Current: current,
		Oldest:  current - dataUsageUpdateDirCycles,
	}
	if current < dataUsageUpdateDirCycles {
		req.Oldest = 0
	}

	// Load initial state from local...
	var bf *bloomFilter
	bfr, err := intDataUpdateTracker.cycleFilter(ctx, req)
	logger.LogIf(ctx, err)
	if err == nil && bfr.Complete {
		nbf := intDataUpdateTracker.newBloomFilter()
		bf = &nbf
		_, err = bf.ReadFrom(bytes.NewReader(bfr.Filter))
		logger.LogIf(ctx, err)
	}

	var mu sync.Mutex
	g := errgroup.WithNErrs(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		g.Go(func() error {
			serverBF, err := client.cycleServerBloomFilter(ctx, req)
			if false && intDataUpdateTracker.debug {
				b, _ := json.MarshalIndent(serverBF, "", "  ")
				logger.Info("Drive %v, Bloom filter: %v", client.host.Name, string(b))
			}
			// Keep lock while checking result.
			mu.Lock()
			defer mu.Unlock()

			if err != nil || !serverBF.Complete || bf == nil {
				logger.LogOnceIf(ctx, err, client.host.String(), client.cycleServerBloomFilter)
				bf = nil
				return nil
			}

			var tmp bloom.BloomFilter
			_, err = tmp.ReadFrom(bytes.NewReader(serverBF.Filter))
			if err != nil {
				logger.LogIf(ctx, err)
				bf = nil
				return nil
			}
			if bf.BloomFilter == nil {
				bf.BloomFilter = &tmp
			} else {
				err = bf.Merge(&tmp)
				if err != nil {
					logger.LogIf(ctx, err)
					bf = nil
					return nil
				}
			}
			return nil
		}, idx)
	}
	g.Wait()
	return bf, nil
}

var errPeerNotReachable = errors.New("peer is not reachable")

// GetLocks - makes GetLocks RPC call on all peers.
func (sys *NotificationSys) GetLocks(ctx context.Context, r *http.Request) []*PeerLocks {
	locksResp := make([]*PeerLocks, len(sys.peerClients))
	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		index := index
		g.Go(func() error {
			if client == nil {
				return errPeerNotReachable
			}
			serverLocksResp, err := sys.peerClients[index].GetLocks()
			if err != nil {
				return err
			}
			locksResp[index] = &PeerLocks{
				Addr:  sys.peerClients[index].host.String(),
				Locks: serverLocksResp,
			}
			return nil
		}, index)
	}
	for index, err := range g.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress",
			sys.peerClients[index].host.String())
		ctx := logger.SetReqInfo(ctx, reqInfo)
		logger.LogOnceIf(ctx, err, sys.peerClients[index].host.String())
	}
	locksResp = append(locksResp, &PeerLocks{
		Addr:  getHostName(r),
		Locks: globalLockServer.DupLockMap(),
	})
	return locksResp
}

// LoadBucketMetadata - calls LoadBucketMetadata call on all peers
func (sys *NotificationSys) LoadBucketMetadata(ctx context.Context, bucketName string) {
	if globalIsGateway {
		return
	}

	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.LoadBucketMetadata(bucketName)
		}, idx, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}
}

// DeleteBucketMetadata - calls DeleteBucketMetadata call on all peers
func (sys *NotificationSys) DeleteBucketMetadata(ctx context.Context, bucketName string) {
	globalReplicationStats.Delete(bucketName)
	globalBucketMetadataSys.Remove(bucketName)
	globalBucketTargetSys.Delete(bucketName)
	globalNotificationSys.RemoveNotification(bucketName)
	globalBucketConnStats.delete(bucketName)
	if localMetacacheMgr != nil {
		localMetacacheMgr.deleteBucketCache(bucketName)
	}

	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.DeleteBucketMetadata(bucketName)
		}, idx, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}
}

// GetClusterAllBucketStats - returns bucket stats for all buckets from all remote peers.
func (sys *NotificationSys) GetClusterAllBucketStats(ctx context.Context) []BucketStatsMap {
	ng := WithNPeers(len(sys.peerClients))
	replicationStats := make([]BucketStatsMap, len(sys.peerClients))
	for index, client := range sys.peerClients {
		index := index
		client := client
		ng.Go(ctx, func() error {
			if client == nil {
				return errPeerNotReachable
			}
			bsMap, err := client.GetAllBucketStats()
			if err != nil {
				return err
			}
			replicationStats[index] = bsMap
			return nil
		}, index, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}

	replicationStatsList := globalReplicationStats.GetAll()
	bucketStatsMap := make(map[string]BucketStats, len(replicationStatsList))
	for k, replicationStats := range replicationStatsList {
		bucketStatsMap[k] = BucketStats{
			ReplicationStats: replicationStats,
		}
	}

	replicationStats = append(replicationStats, BucketStatsMap(bucketStatsMap))
	return replicationStats
}

// GetClusterBucketStats - calls GetClusterBucketStats call on all peers for a cluster statistics view.
func (sys *NotificationSys) GetClusterBucketStats(ctx context.Context, bucketName string) []BucketStats {
	ng := WithNPeers(len(sys.peerClients))
	bucketStats := make([]BucketStats, len(sys.peerClients))
	for index, client := range sys.peerClients {
		index := index
		client := client
		ng.Go(ctx, func() error {
			if client == nil {
				return errPeerNotReachable
			}
			bs, err := client.GetBucketStats(bucketName)
			if err != nil {
				return err
			}
			bucketStats[index] = bs
			return nil
		}, index, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}
	bucketStats = append(bucketStats, BucketStats{
		ReplicationStats: globalReplicationStats.Get(bucketName),
	})
	return bucketStats
}

// ReloadPoolMeta reloads on disk updates on pool metadata
func (sys *NotificationSys) ReloadPoolMeta(ctx context.Context) {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.ReloadPoolMeta(ctx)
		}, idx, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}
}

// LoadTransitionTierConfig notifies remote peers to load their remote tier
// configs from config store.
func (sys *NotificationSys) LoadTransitionTierConfig(ctx context.Context) {
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(ctx, func() error {
			return client.LoadTransitionTierConfig(ctx)
		}, idx, *client.host)
	}
	for _, nErr := range ng.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", nErr.Host.String())
		if nErr.Err != nil {
			logger.LogIf(logger.SetReqInfo(ctx, reqInfo), nErr.Err)
		}
	}
}

// Loads notification policies for all buckets into NotificationSys.
func (sys *NotificationSys) set(bucket BucketInfo, meta BucketMetadata) {
	config := meta.notificationConfig
	if config == nil {
		return
	}
	config.SetRegion(globalSite.Region)
	if err := config.Validate(globalSite.Region, globalNotificationSys.targetList); err != nil {
		if _, ok := err.(*event.ErrARNNotFound); !ok {
			logger.LogIf(GlobalContext, err)
		}
	}
	sys.AddRulesMap(bucket.Name, config.ToRulesMap())
}

// InitBucketTargets - initializes notification system from notification.xml of all buckets.
func (sys *NotificationSys) InitBucketTargets(ctx context.Context, objAPI ObjectLayer) error {
	if objAPI == nil {
		return errServerNotInitialized
	}

	// In gateway mode, notifications are not supported - except NAS gateway.
	if globalIsGateway && !objAPI.IsNotificationSupported() {
		return nil
	}

	logger.LogIf(ctx, sys.targetList.Add(globalConfigTargetList.Targets()...))

	go func() {
		for res := range sys.targetResCh {
			if res.Err != nil {
				reqInfo := &logger.ReqInfo{}
				reqInfo.AppendTags("targetID", res.ID.Name)
				logger.LogOnceIf(logger.SetReqInfo(GlobalContext, reqInfo), res.Err, res.ID.String())
			}
		}
	}()

	return nil
}

// AddRulesMap - adds rules map for bucket name.
func (sys *NotificationSys) AddRulesMap(bucketName string, rulesMap event.RulesMap) {
	sys.Lock()
	defer sys.Unlock()

	rulesMap = rulesMap.Clone()

	for _, targetRulesMap := range sys.bucketRemoteTargetRulesMap[bucketName] {
		rulesMap.Add(targetRulesMap)
	}

	// Do not add for an empty rulesMap.
	if len(rulesMap) == 0 {
		delete(sys.bucketRulesMap, bucketName)
	} else {
		sys.bucketRulesMap[bucketName] = rulesMap
	}
}

// RemoveRulesMap - removes rules map for bucket name.
func (sys *NotificationSys) RemoveRulesMap(bucketName string, rulesMap event.RulesMap) {
	sys.Lock()
	defer sys.Unlock()

	sys.bucketRulesMap[bucketName].Remove(rulesMap)
	if len(sys.bucketRulesMap[bucketName]) == 0 {
		delete(sys.bucketRulesMap, bucketName)
	}
}

// ConfiguredTargetIDs - returns list of configured target id's
func (sys *NotificationSys) ConfiguredTargetIDs() []event.TargetID {
	if sys == nil {
		return nil
	}

	sys.RLock()
	defer sys.RUnlock()

	var targetIDs []event.TargetID
	for _, rmap := range sys.bucketRulesMap {
		for _, rules := range rmap {
			for _, targetSet := range rules {
				for id := range targetSet {
					targetIDs = append(targetIDs, id)
				}
			}
		}
	}
	// Filter out targets configured via env
	var tIDs []event.TargetID
	for _, targetID := range targetIDs {
		if !globalEnvTargetList.Exists(targetID) {
			tIDs = append(tIDs, targetID)
		}
	}
	return tIDs
}

// RemoveNotification - removes all notification configuration for bucket name.
func (sys *NotificationSys) RemoveNotification(bucketName string) {
	sys.Lock()
	defer sys.Unlock()

	delete(sys.bucketRulesMap, bucketName)

	targetIDSet := event.NewTargetIDSet()
	for targetID := range sys.bucketRemoteTargetRulesMap[bucketName] {
		targetIDSet[targetID] = struct{}{}
		delete(sys.bucketRemoteTargetRulesMap[bucketName], targetID)
	}
	sys.targetList.Remove(targetIDSet)

	delete(sys.bucketRemoteTargetRulesMap, bucketName)
}

// RemoveAllRemoteTargets - closes and removes all notification targets.
func (sys *NotificationSys) RemoveAllRemoteTargets() {
	sys.Lock()
	defer sys.Unlock()

	for _, targetMap := range sys.bucketRemoteTargetRulesMap {
		targetIDSet := event.NewTargetIDSet()
		for k := range targetMap {
			targetIDSet[k] = struct{}{}
		}
		sys.targetList.Remove(targetIDSet)
	}
}

// Send - sends event data to all matching targets.
func (sys *NotificationSys) Send(args eventArgs) {
	sys.RLock()
	targetIDSet := sys.bucketRulesMap[args.BucketName].Match(args.EventName, args.Object.Name)
	sys.RUnlock()

	if len(targetIDSet) == 0 {
		return
	}

	sys.targetList.Send(args.ToEvent(true), targetIDSet, sys.targetResCh)
}

// GetCPUs - Get all CPU information.
func (sys *NotificationSys) GetCPUs(ctx context.Context) []madmin.CPUs {
	reply := make([]madmin.CPUs, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetCPUs(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetPartitions - Disk partition information
func (sys *NotificationSys) GetPartitions(ctx context.Context) []madmin.Partitions {
	reply := make([]madmin.Partitions, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetPartitions(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetOSInfo - Get operating system's information
func (sys *NotificationSys) GetOSInfo(ctx context.Context) []madmin.OSInfo {
	reply := make([]madmin.OSInfo, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetOSInfo(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetMetrics - Get metrics from all peers.
func (sys *NotificationSys) GetMetrics(ctx context.Context, t madmin.MetricType, hosts map[string]struct{}) []madmin.RealtimeMetrics {
	reply := make([]madmin.RealtimeMetrics, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		host := client.host.String()
		if len(hosts) > 0 {
			if _, ok := hosts[host]; !ok {
				continue
			}
		}

		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetMetrics(ctx, t)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			reply[index].Errors = []string{err.Error()}
		}
	}
	return reply
}

// GetSysConfig - Get information about system config
// (only the config that are of concern to minio)
func (sys *NotificationSys) GetSysConfig(ctx context.Context) []madmin.SysConfig {
	reply := make([]madmin.SysConfig, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetSysConfig(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetSysServices - Get information about system services
// (only the services that are of concern to minio)
func (sys *NotificationSys) GetSysServices(ctx context.Context) []madmin.SysServices {
	reply := make([]madmin.SysServices, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetSELinuxInfo(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

func (sys *NotificationSys) addNodeErr(nodeInfo madmin.NodeInfo, peerClient *peerRESTClient, err error) {
	addr := peerClient.host.String()
	reqInfo := (&logger.ReqInfo{}).AppendTags("remotePeer", addr)
	ctx := logger.SetReqInfo(GlobalContext, reqInfo)
	logger.LogIf(ctx, err)
	nodeInfo.SetAddr(addr)
	nodeInfo.SetError(err.Error())
}

// GetSysErrors - Memory information
func (sys *NotificationSys) GetSysErrors(ctx context.Context) []madmin.SysErrors {
	reply := make([]madmin.SysErrors, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetSysErrors(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetMemInfo - Memory information
func (sys *NotificationSys) GetMemInfo(ctx context.Context) []madmin.MemInfo {
	reply := make([]madmin.MemInfo, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetMemInfo(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

// GetProcInfo - Process information
func (sys *NotificationSys) GetProcInfo(ctx context.Context) []madmin.ProcInfo {
	reply := make([]madmin.ProcInfo, len(sys.peerClients))

	g := errgroup.WithNErrs(len(sys.peerClients))
	for index, client := range sys.peerClients {
		if client == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reply[index], err = sys.peerClients[index].GetProcInfo(ctx)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		if err != nil {
			sys.addNodeErr(&reply[index], sys.peerClients[index], err)
		}
	}
	return reply
}

func getOfflineDisks(offlineHost string, endpoints EndpointServerPools) []madmin.Disk {
	var offlineDisks []madmin.Disk
	for _, pool := range endpoints {
		for _, ep := range pool.Endpoints {
			if offlineHost == ep.Host {
				offlineDisks = append(offlineDisks, madmin.Disk{
					Endpoint: ep.String(),
					State:    string(madmin.ItemOffline),
				})
			}
		}
	}
	return offlineDisks
}

// ServerInfo - calls ServerInfo RPC call on all peers.
func (sys *NotificationSys) ServerInfo() []madmin.ServerProperties {
	reply := make([]madmin.ServerProperties, len(sys.peerClients))
	var wg sync.WaitGroup
	for i, client := range sys.peerClients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(client *peerRESTClient, idx int) {
			defer wg.Done()
			info, err := client.ServerInfo()
			if err != nil {
				info.Endpoint = client.host.String()
				info.State = string(madmin.ItemOffline)
				info.Disks = getOfflineDisks(info.Endpoint, globalEndpoints)
			} else {
				info.State = string(madmin.ItemOnline)
			}
			reply[idx] = info
		}(client, i)
	}
	wg.Wait()

	return reply
}

// GetLocalDiskIDs - return disk ids of the local disks of the peers.
func (sys *NotificationSys) GetLocalDiskIDs(ctx context.Context) (localDiskIDs [][]string) {
	localDiskIDs = make([][]string, len(sys.peerClients))
	var wg sync.WaitGroup
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, client *peerRESTClient) {
			defer wg.Done()
			localDiskIDs[idx] = client.GetLocalDiskIDs(ctx)
		}(idx, client)
	}
	wg.Wait()
	return localDiskIDs
}

// returns all the peers that are currently online.
func (sys *NotificationSys) getOnlinePeers() []*peerRESTClient {
	var peerClients []*peerRESTClient
	for _, peerClient := range sys.allPeerClients {
		if peerClient != nil && peerClient.IsOnline() {
			peerClients = append(peerClients, peerClient)
		}
	}
	return peerClients
}

// restClientFromHash will return a deterministic peerRESTClient based on s.
// Will return nil if client is local.
func (sys *NotificationSys) restClientFromHash(s string) (client *peerRESTClient) {
	if len(sys.peerClients) == 0 {
		return nil
	}
	peerClients := sys.getOnlinePeers()
	if len(peerClients) == 0 {
		return nil
	}
	idx := xxhash.Sum64String(s) % uint64(len(peerClients))
	return peerClients[idx]
}

// GetPeerOnlineCount gets the count of online and offline nodes.
func (sys *NotificationSys) GetPeerOnlineCount() (nodesOnline, nodesOffline int) {
	nodesOnline = 1 // Self is always online.
	nodesOffline = 0
	nodesOnlineIndex := make([]bool, len(sys.peerClients))
	var wg sync.WaitGroup
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, client *peerRESTClient) {
			defer wg.Done()
			nodesOnlineIndex[idx] = client.restClient.HealthCheckFn()
		}(idx, client)

	}
	wg.Wait()

	for _, online := range nodesOnlineIndex {
		if online {
			nodesOnline++
		} else {
			nodesOffline++
		}
	}
	return
}

// NewNotificationSys - creates new notification system object.
func NewNotificationSys(endpoints EndpointServerPools) *NotificationSys {
	// targetList/bucketRulesMap/bucketRemoteTargetRulesMap are populated by NotificationSys.Init()
	remote, all := newPeerRestClients(endpoints)
	return &NotificationSys{
		targetList:                 event.NewTargetList(),
		targetResCh:                make(chan event.TargetIDResult),
		bucketRulesMap:             make(map[string]event.RulesMap),
		bucketRemoteTargetRulesMap: make(map[string]map[event.TargetID]event.RulesMap),
		peerClients:                remote,
		allPeerClients:             all,
	}
}

type eventArgs struct {
	EventName    event.Name
	BucketName   string
	Object       ObjectInfo
	ReqParams    map[string]string
	RespElements map[string]string
	Host         string
	UserAgent    string
}

// ToEvent - converts to notification event.
func (args eventArgs) ToEvent(escape bool) event.Event {
	eventTime := UTCNow()
	uniqueID := fmt.Sprintf("%X", eventTime.UnixNano())

	respElements := map[string]string{
		"x-amz-request-id": args.RespElements["requestId"],
		"x-minio-origin-endpoint": func() string {
			if globalMinioEndpoint != "" {
				return globalMinioEndpoint
			}
			return getAPIEndpoints()[0]
		}(), // MinIO specific custom elements.
	}
	// Add deployment as part of
	if globalDeploymentID != "" {
		respElements["x-minio-deployment-id"] = globalDeploymentID
	}
	if args.RespElements["content-length"] != "" {
		respElements["content-length"] = args.RespElements["content-length"]
	}
	keyName := args.Object.Name
	if escape {
		keyName = url.QueryEscape(args.Object.Name)
	}
	newEvent := event.Event{
		EventVersion:      "2.0",
		EventSource:       "minio:s3",
		AwsRegion:         args.ReqParams["region"],
		EventTime:         eventTime.Format(event.AMZTimeFormat),
		EventName:         args.EventName,
		UserIdentity:      event.Identity{PrincipalID: args.ReqParams["principalId"]},
		RequestParameters: args.ReqParams,
		ResponseElements:  respElements,
		S3: event.Metadata{
			SchemaVersion:   "1.0",
			ConfigurationID: "Config",
			Bucket: event.Bucket{
				Name:          args.BucketName,
				OwnerIdentity: event.Identity{PrincipalID: args.ReqParams["principalId"]},
				ARN:           policy.ResourceARNPrefix + args.BucketName,
			},
			Object: event.Object{
				Key:       keyName,
				VersionID: args.Object.VersionID,
				Sequencer: uniqueID,
			},
		},
		Source: event.Source{
			Host:      args.Host,
			UserAgent: args.UserAgent,
		},
	}

	if args.EventName != event.ObjectRemovedDelete && args.EventName != event.ObjectRemovedDeleteMarkerCreated {
		newEvent.S3.Object.ETag = args.Object.ETag
		newEvent.S3.Object.Size = args.Object.Size
		newEvent.S3.Object.ContentType = args.Object.ContentType
		newEvent.S3.Object.UserMetadata = make(map[string]string, len(args.Object.UserDefined))
		for k, v := range args.Object.UserDefined {
			if strings.HasPrefix(strings.ToLower(k), ReservedMetadataPrefixLower) {
				continue
			}
			newEvent.S3.Object.UserMetadata[k] = v
		}
	}

	return newEvent
}

func sendEvent(args eventArgs) {
	args.Object.Size, _ = args.Object.GetActualSize()

	// avoid generating a notification for REPLICA creation event.
	if _, ok := args.ReqParams[xhttp.MinIOSourceReplicationRequest]; ok {
		return
	}
	// remove sensitive encryption entries in metadata.
	crypto.RemoveSensitiveEntries(args.Object.UserDefined)
	crypto.RemoveInternalEntries(args.Object.UserDefined)

	// globalNotificationSys is not initialized in gateway mode.
	if globalNotificationSys == nil {
		return
	}
	if globalHTTPListen.NumSubscribers(args.EventName) > 0 {
		globalHTTPListen.Publish(args.ToEvent(false))
	}

	globalNotificationSys.Send(args)
}

// GetBandwidthReports - gets the bandwidth report from all nodes including self.
func (sys *NotificationSys) GetBandwidthReports(ctx context.Context, buckets ...string) madmin.BucketBandwidthReport {
	reports := make([]*madmin.BucketBandwidthReport, len(sys.peerClients))
	g := errgroup.WithNErrs(len(sys.peerClients))
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			reports[index], err = sys.peerClients[index].MonitorBandwidth(ctx, buckets)
			return err
		}, index)
	}

	for index, err := range g.Wait() {
		reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress",
			sys.peerClients[index].host.String())
		ctx := logger.SetReqInfo(ctx, reqInfo)
		logger.LogOnceIf(ctx, err, sys.peerClients[index].host.String())
	}
	reports = append(reports, globalBucketMonitor.GetReport(bucketBandwidth.SelectBuckets(buckets...)))
	consolidatedReport := madmin.BucketBandwidthReport{
		BucketStats: make(map[string]madmin.BandwidthDetails),
	}
	for _, report := range reports {
		if report == nil || report.BucketStats == nil {
			continue
		}
		for bucket := range report.BucketStats {
			d, ok := consolidatedReport.BucketStats[bucket]
			if !ok {
				consolidatedReport.BucketStats[bucket] = madmin.BandwidthDetails{}
				d = consolidatedReport.BucketStats[bucket]
				d.LimitInBytesPerSecond = report.BucketStats[bucket].LimitInBytesPerSecond
			}
			if d.LimitInBytesPerSecond < report.BucketStats[bucket].LimitInBytesPerSecond {
				d.LimitInBytesPerSecond = report.BucketStats[bucket].LimitInBytesPerSecond
			}
			d.CurrentBandwidthInBytesPerSecond += report.BucketStats[bucket].CurrentBandwidthInBytesPerSecond
			consolidatedReport.BucketStats[bucket] = d
		}
	}
	return consolidatedReport
}

// GetClusterMetrics - gets the cluster metrics from all nodes excluding self.
func (sys *NotificationSys) GetClusterMetrics(ctx context.Context) <-chan Metric {
	if sys == nil {
		return nil
	}
	g := errgroup.WithNErrs(len(sys.peerClients))
	peerChannels := make([]<-chan Metric, len(sys.peerClients))
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		index := index
		g.Go(func() error {
			var err error
			peerChannels[index], err = sys.peerClients[index].GetPeerMetrics(ctx)
			return err
		}, index)
	}

	ch := make(chan Metric)
	var wg sync.WaitGroup
	for index, err := range g.Wait() {
		if err != nil {
			if sys.peerClients[index] != nil {
				reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress",
					sys.peerClients[index].host.String())
				logger.LogOnceIf(logger.SetReqInfo(ctx, reqInfo), err, sys.peerClients[index].host.String())
			} else {
				logger.LogOnceIf(ctx, err, "peer-offline")
			}
			continue
		}
		wg.Add(1)
		go func(ctx context.Context, peerChannel <-chan Metric, wg *sync.WaitGroup) {
			defer wg.Done()
			for {
				select {
				case m, ok := <-peerChannel:
					if !ok {
						return
					}
					ch <- m
				case <-ctx.Done():
					return
				}
			}
		}(ctx, peerChannels[index], &wg)
	}
	go func(wg *sync.WaitGroup, ch chan Metric) {
		wg.Wait()
		close(ch)
	}(&wg, ch)
	return ch
}

// ServiceFreeze freezes all S3 API calls when 'freeze' is true,
// 'freeze' is 'false' would resume all S3 API calls again.
// NOTE: once a tenant is frozen either two things needs to
// happen before resuming normal operations.
// - Server needs to be restarted 'mc admin service restart'
// - 'freeze' should be set to 'false' for this call
//   to resume normal operations.
func (sys *NotificationSys) ServiceFreeze(ctx context.Context, freeze bool) []NotificationPeerErr {
	serviceSig := serviceUnFreeze
	if freeze {
		serviceSig = serviceFreeze
	}
	ng := WithNPeers(len(sys.peerClients))
	for idx, client := range sys.peerClients {
		if client == nil {
			continue
		}
		client := client
		ng.Go(GlobalContext, func() error {
			return client.SignalService(serviceSig, "")
		}, idx, *client.host)
	}
	nerrs := ng.Wait()
	if freeze {
		freezeServices()
	} else {
		unfreezeServices()
	}
	return nerrs
}

// Netperf - perform mesh style network throughput test
func (sys *NotificationSys) Netperf(ctx context.Context, duration time.Duration) []madmin.NetperfNodeResult {
	length := len(sys.allPeerClients)
	if length == 0 {
		// For single node erasure setup.
		return nil
	}
	results := make([]madmin.NetperfNodeResult, length)

	scheme := "http"
	if globalIsTLS {
		scheme = "https"
	}

	var wg sync.WaitGroup
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			r, err := sys.peerClients[index].Netperf(ctx, duration)
			u := &url.URL{
				Scheme: scheme,
				Host:   sys.peerClients[index].host.String(),
			}
			if err != nil {
				results[index].Error = err.Error()
			} else {
				results[index] = r
			}
			results[index].Endpoint = u.String()
		}(index)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r := netperf(ctx, duration)
		u := &url.URL{
			Scheme: scheme,
			Host:   globalLocalNodeName,
		}
		results[len(results)-1] = r
		results[len(results)-1].Endpoint = u.String()
	}()
	wg.Wait()

	return results
}

// SpeedTest run GET/PUT tests at input concurrency for requested object size,
// optionally you can extend the tests longer with time.Duration.
func (sys *NotificationSys) SpeedTest(ctx context.Context, sopts speedTestOpts) []SpeedTestResult {
	length := len(sys.allPeerClients)
	if length == 0 {
		// For single node erasure setup.
		length = 1
	}
	results := make([]SpeedTestResult, length)

	scheme := "http"
	if globalIsTLS {
		scheme = "https"
	}

	var wg sync.WaitGroup
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			r, err := sys.peerClients[index].SpeedTest(ctx, sopts)
			u := &url.URL{
				Scheme: scheme,
				Host:   sys.peerClients[index].host.String(),
			}
			if err != nil {
				results[index].Error = err.Error()
			} else {
				results[index] = r
			}
			results[index].Endpoint = u.String()
		}(index)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r, err := selfSpeedTest(ctx, sopts)
		u := &url.URL{
			Scheme: scheme,
			Host:   globalLocalNodeName,
		}
		if err != nil {
			results[len(results)-1].Error = err.Error()
		} else {
			results[len(results)-1] = r
		}
		results[len(results)-1].Endpoint = u.String()
	}()
	wg.Wait()

	return results
}

// DriveSpeedTest - Drive performance information
func (sys *NotificationSys) DriveSpeedTest(ctx context.Context, opts madmin.DriveSpeedTestOpts) chan madmin.DriveSpeedTestResult {
	ch := make(chan madmin.DriveSpeedTestResult)
	var wg sync.WaitGroup
	for _, client := range sys.peerClients {
		if client == nil {
			continue
		}
		wg.Add(1)
		go func(client *peerRESTClient) {
			defer wg.Done()
			resp, err := client.DriveSpeedTest(ctx, opts)
			if err != nil {
				resp.Error = err.Error()
			}

			select {
			case <-ctx.Done():
			case ch <- resp:
			}

			reqInfo := (&logger.ReqInfo{}).AppendTags("remotePeer", client.host.String())
			ctx := logger.SetReqInfo(GlobalContext, reqInfo)
			logger.LogIf(ctx, err)
		}(client)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		case ch <- driveSpeedTest(ctx, opts):
		}
	}()

	go func(wg *sync.WaitGroup, ch chan madmin.DriveSpeedTestResult) {
		wg.Wait()
		close(ch)
	}(&wg, ch)

	return ch
}

// ReloadSiteReplicationConfig - tells all peer minio nodes to reload the
// site-replication configuration.
func (sys *NotificationSys) ReloadSiteReplicationConfig(ctx context.Context) []error {
	errs := make([]error, len(sys.allPeerClients))
	var wg sync.WaitGroup
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = sys.peerClients[index].ReloadSiteReplicationConfig(ctx)
		}(index)
	}

	wg.Wait()
	return errs
}

// GetLastDayTierStats fetches per-tier stats of the last 24hrs from all peers
func (sys *NotificationSys) GetLastDayTierStats(ctx context.Context) DailyAllTierStats {
	errs := make([]error, len(sys.allPeerClients))
	lastDayStats := make([]DailyAllTierStats, len(sys.allPeerClients))
	var wg sync.WaitGroup
	for index := range sys.peerClients {
		if sys.peerClients[index] == nil {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			lastDayStats[index], errs[index] = sys.peerClients[index].GetLastDayTierStats(ctx)
		}(index)
	}

	wg.Wait()
	merged := globalTransitionState.getDailyAllTierStats()
	for i, stat := range lastDayStats {
		if errs[i] != nil {
			logger.LogIf(ctx, fmt.Errorf("failed to fetch last day tier stats: %w", errs[i]))
			continue
		}
		merged.merge(stat)
	}
	return merged
}
