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

const (
	peerRESTVersion       = "v24" // Change ServerUpdate to DownloadBinary and CommitBinary
	peerRESTVersionPrefix = SlashSeparator + peerRESTVersion
	peerRESTPrefix        = minioReservedBucketPath + "/peer"
	peerRESTPath          = peerRESTPrefix + peerRESTVersionPrefix
)

const (
	peerRESTMethodHealth                      = "/health"
	peerRESTMethodServerInfo                  = "/serverinfo"
	peerRESTMethodCPUInfo                     = "/cpuinfo"
	peerRESTMethodDiskHwInfo                  = "/diskhwinfo"
	peerRESTMethodOsInfo                      = "/osinfo"
	peerRESTMethodMemInfo                     = "/meminfo"
	peerRESTMethodProcInfo                    = "/procinfo"
	peerRESTMethodSysErrors                   = "/syserrors"
	peerRESTMethodSysServices                 = "/sysservices"
	peerRESTMethodSysConfig                   = "/sysconfig"
	peerRESTMethodDeleteBucketMetadata        = "/deletebucketmetadata"
	peerRESTMethodLoadBucketMetadata          = "/loadbucketmetadata"
	peerRESTMethodGetBucketStats              = "/getbucketstats"
	peerRESTMethodGetAllBucketStats           = "/getallbucketstats"
	peerRESTMethodDownloadBinary              = "/downloadbinary"
	peerRESTMethodCommitBinary                = "/commitbinary"
	peerRESTMethodSignalService               = "/signalservice"
	peerRESTMethodBackgroundHealStatus        = "/backgroundhealstatus"
	peerRESTMethodGetLocks                    = "/getlocks"
	peerRESTMethodLoadUser                    = "/loaduser"
	peerRESTMethodLoadServiceAccount          = "/loadserviceaccount"
	peerRESTMethodDeleteUser                  = "/deleteuser"
	peerRESTMethodDeleteServiceAccount        = "/deleteserviceaccount"
	peerRESTMethodLoadPolicy                  = "/loadpolicy"
	peerRESTMethodLoadPolicyMapping           = "/loadpolicymapping"
	peerRESTMethodDeletePolicy                = "/deletepolicy"
	peerRESTMethodLoadGroup                   = "/loadgroup"
	peerRESTMethodStartProfiling              = "/startprofiling"
	peerRESTMethodDownloadProfilingData       = "/downloadprofilingdata"
	peerRESTMethodCycleBloom                  = "/cyclebloom"
	peerRESTMethodTrace                       = "/trace"
	peerRESTMethodListen                      = "/listen"
	peerRESTMethodLog                         = "/log"
	peerRESTMethodGetLocalDiskIDs             = "/getlocaldiskids"
	peerRESTMethodGetBandwidth                = "/bandwidth"
	peerRESTMethodGetMetacacheListing         = "/getmetacache"
	peerRESTMethodUpdateMetacacheListing      = "/updatemetacache"
	peerRESTMethodGetPeerMetrics              = "/peermetrics"
	peerRESTMethodLoadTransitionTierConfig    = "/loadtransitiontierconfig"
	peerRESTMethodSpeedTest                   = "/speedtest"
	peerRESTMethodDriveSpeedTest              = "/drivespeedtest"
	peerRESTMethodReloadSiteReplicationConfig = "/reloadsitereplicationconfig"
	peerRESTMethodReloadPoolMeta              = "/reloadpoolmeta"
	peerRESTMethodGetLastDayTierStats         = "/getlastdaytierstats"
	peerRESTMethodDevNull                     = "/devnull"
	peerRESTMethodNetperf                     = "/netperf"
	peerRESTMethodMetrics                     = "/metrics"
)

const (
	peerRESTBucket       = "bucket"
	peerRESTBuckets      = "buckets"
	peerRESTUser         = "user"
	peerRESTGroup        = "group"
	peerRESTUserTemp     = "user-temp"
	peerRESTPolicy       = "policy"
	peerRESTUserOrGroup  = "user-or-group"
	peerRESTIsGroup      = "is-group"
	peerRESTSignal       = "signal"
	peerRESTSubSys       = "sub-sys"
	peerRESTProfiler     = "profiler"
	peerRESTSize         = "size"
	peerRESTConcurrent   = "concurrent"
	peerRESTDuration     = "duration"
	peerRESTStorageClass = "storage-class"
	peerRESTTypes        = "types"

	peerRESTListenBucket = "bucket"
	peerRESTListenPrefix = "prefix"
	peerRESTListenSuffix = "suffix"
	peerRESTListenEvents = "events"
)
