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
	"github.com/minio/madmin-go"
)

// GetTotalCapacity gets the total capacity in the cluster.
func GetTotalCapacity(diskInfo []madmin.Disk) (capacity uint64) {
	for _, disk := range diskInfo {
		capacity += disk.TotalSpace
	}
	return
}

// GetTotalUsableCapacity gets the total usable capacity in the cluster.
func GetTotalUsableCapacity(diskInfo []madmin.Disk, s StorageInfo) (capacity uint64) {
	if globalIsGateway {
		return 0
	}
	for _, disk := range diskInfo {
		// Ignore parity disks
		if disk.DiskIndex < s.Backend.StandardSCData[disk.PoolIndex] {
			capacity += disk.TotalSpace
		}
	}
	return
}

// GetTotalCapacityFree gets the total capacity free in the cluster.
func GetTotalCapacityFree(diskInfo []madmin.Disk) (capacity uint64) {
	for _, d := range diskInfo {
		capacity += d.AvailableSpace
	}
	return
}

// GetTotalUsableCapacityFree gets the total usable capacity free in the cluster.
func GetTotalUsableCapacityFree(diskInfo []madmin.Disk, s StorageInfo) (capacity uint64) {
	if globalIsGateway {
		return 0
	}

	for _, disk := range diskInfo {
		// Ignore parity disks
		if disk.DiskIndex < s.Backend.StandardSCData[disk.PoolIndex] {
			capacity += disk.AvailableSpace
		}
	}
	return
}
