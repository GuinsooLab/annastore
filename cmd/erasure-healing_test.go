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
	"crypto/rand"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/minio/madmin-go"
)

// Tests both object and bucket healing.
func TestHealing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	obj, fsDirs, err := prepareErasure16(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Shutdown(context.Background())
	defer removeRoots(fsDirs)

	z := obj.(*erasureServerPools)
	er := z.serverPools[0].sets[0]

	// Create "bucket"
	err = obj.MakeBucketWithLocation(ctx, "bucket", BucketOptions{})
	if err != nil {
		t.Fatal(err)
	}

	bucket := "bucket"
	object := "object"

	data := make([]byte, 1*humanize.MiByte)
	length := int64(len(data))
	_, err = rand.Read(data)
	if err != nil {
		t.Fatal(err)
	}

	_, err = obj.PutObject(ctx, bucket, object, mustGetPutObjReader(t, bytes.NewReader(data), length, "", ""), ObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}

	disk := er.getDisks()[0]
	fileInfoPreHeal, err := disk.ReadVersion(context.Background(), bucket, object, "", false)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the object - to simulate the case where the disk was down when the object
	// was created.
	err = removeAll(pathJoin(disk.String(), bucket, object))
	if err != nil {
		t.Fatal(err)
	}

	_, err = er.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealNormalScan})
	if err != nil {
		t.Fatal(err)
	}

	fileInfoPostHeal, err := disk.ReadVersion(context.Background(), bucket, object, "", false)
	if err != nil {
		t.Fatal(err)
	}

	// After heal the meta file should be as expected.
	if !reflect.DeepEqual(fileInfoPreHeal, fileInfoPostHeal) {
		t.Fatal("HealObject failed")
	}

	err = os.RemoveAll(path.Join(fsDirs[0], bucket, object, "er.meta"))
	if err != nil {
		t.Fatal(err)
	}

	// Write er.meta with different modtime to simulate the case where a disk had
	// gone down when an object was replaced by a new object.
	fileInfoOutDated := fileInfoPreHeal
	fileInfoOutDated.ModTime = time.Now()
	err = disk.WriteMetadata(context.Background(), bucket, object, fileInfoOutDated)
	if err != nil {
		t.Fatal(err)
	}

	_, err = er.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealDeepScan})
	if err != nil {
		t.Fatal(err)
	}

	fileInfoPostHeal, err = disk.ReadVersion(context.Background(), bucket, object, "", false)
	if err != nil {
		t.Fatal(err)
	}

	// After heal the meta file should be as expected.
	if !reflect.DeepEqual(fileInfoPreHeal, fileInfoPostHeal) {
		t.Fatal("HealObject failed")
	}

	// Remove the bucket - to simulate the case where bucket was
	// created when the disk was down.
	err = os.RemoveAll(path.Join(fsDirs[0], bucket))
	if err != nil {
		t.Fatal(err)
	}
	// This would create the bucket.
	_, err = er.HealBucket(ctx, bucket, madmin.HealOpts{
		DryRun: false,
		Remove: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Stat the bucket to make sure that it was created.
	_, err = er.getDisks()[0].StatVol(context.Background(), bucket)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealObjectCorrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resetGlobalHealState()
	defer resetGlobalHealState()

	nDisks := 16
	fsDirs, err := getRandomDisks(nDisks)
	if err != nil {
		t.Fatal(err)
	}

	defer removeRoots(fsDirs)

	// Everything is fine, should return nil
	objLayer, _, err := initObjectLayer(ctx, mustGetPoolEndpoints(fsDirs...))
	if err != nil {
		t.Fatal(err)
	}

	bucket := "bucket"
	object := "object"
	data := bytes.Repeat([]byte("a"), 5*1024*1024)
	var opts ObjectOptions

	err = objLayer.MakeBucketWithLocation(ctx, bucket, BucketOptions{})
	if err != nil {
		t.Fatalf("Failed to make a bucket - %v", err)
	}

	// Create an object with multiple parts uploaded in decreasing
	// part number.
	uploadID, err := objLayer.NewMultipartUpload(ctx, bucket, object, opts)
	if err != nil {
		t.Fatalf("Failed to create a multipart upload - %v", err)
	}

	var uploadedParts []CompletePart
	for _, partID := range []int{2, 1} {
		pInfo, err1 := objLayer.PutObjectPart(ctx, bucket, object, uploadID, partID, mustGetPutObjReader(t, bytes.NewReader(data), int64(len(data)), "", ""), opts)
		if err1 != nil {
			t.Fatalf("Failed to upload a part - %v", err1)
		}
		uploadedParts = append(uploadedParts, CompletePart{
			PartNumber: pInfo.PartNumber,
			ETag:       pInfo.ETag,
		})
	}

	_, err = objLayer.CompleteMultipartUpload(ctx, bucket, object, uploadID, uploadedParts, ObjectOptions{})
	if err != nil {
		t.Fatalf("Failed to complete multipart upload - %v", err)
	}

	// Test 1: Remove the object backend files from the first disk.
	z := objLayer.(*erasureServerPools)
	er := z.serverPools[0].sets[0]
	erasureDisks := er.getDisks()
	firstDisk := erasureDisks[0]
	err = firstDisk.Delete(context.Background(), bucket, pathJoin(object, xlStorageFormatFile), false)
	if err != nil {
		t.Fatalf("Failed to delete a file - %v", err)
	}

	_, err = objLayer.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealNormalScan})
	if err != nil {
		t.Fatalf("Failed to heal object - %v", err)
	}

	fileInfos, errs := readAllFileInfo(ctx, erasureDisks, bucket, object, "", false)
	fi, err := getLatestFileInfo(ctx, fileInfos, errs)
	if err != nil {
		t.Fatalf("Failed to getLatestFileInfo - %v", err)
	}

	if err = firstDisk.CheckFile(context.Background(), bucket, object); err != nil {
		t.Errorf("Expected er.meta file to be present but stat failed - %v", err)
	}

	err = firstDisk.Delete(context.Background(), bucket, pathJoin(object, fi.DataDir, "part.1"), false)
	if err != nil {
		t.Errorf("Failure during deleting part.1 - %v", err)
	}

	err = firstDisk.WriteAll(context.Background(), bucket, pathJoin(object, fi.DataDir, "part.1"), []byte{})
	if err != nil {
		t.Errorf("Failure during creating part.1 - %v", err)
	}

	_, err = objLayer.HealObject(ctx, bucket, object, "", madmin.HealOpts{DryRun: false, Remove: true, ScanMode: madmin.HealDeepScan})
	if err != nil {
		t.Errorf("Expected nil but received %v", err)
	}

	fileInfos, errs = readAllFileInfo(ctx, erasureDisks, bucket, object, "", false)
	nfi, err := getLatestFileInfo(ctx, fileInfos, errs)
	if err != nil {
		t.Fatalf("Failed to getLatestFileInfo - %v", err)
	}

	if !reflect.DeepEqual(fi, nfi) {
		t.Fatalf("FileInfo not equal after healing")
	}

	err = firstDisk.Delete(context.Background(), bucket, pathJoin(object, fi.DataDir, "part.1"), false)
	if err != nil {
		t.Errorf("Failure during deleting part.1 - %v", err)
	}

	bdata := bytes.Repeat([]byte("b"), int(nfi.Size))
	err = firstDisk.WriteAll(context.Background(), bucket, pathJoin(object, fi.DataDir, "part.1"), bdata)
	if err != nil {
		t.Errorf("Failure during creating part.1 - %v", err)
	}

	_, err = objLayer.HealObject(ctx, bucket, object, "", madmin.HealOpts{DryRun: false, Remove: true, ScanMode: madmin.HealDeepScan})
	if err != nil {
		t.Errorf("Expected nil but received %v", err)
	}

	fileInfos, errs = readAllFileInfo(ctx, erasureDisks, bucket, object, "", false)
	nfi, err = getLatestFileInfo(ctx, fileInfos, errs)
	if err != nil {
		t.Fatalf("Failed to getLatestFileInfo - %v", err)
	}

	if !reflect.DeepEqual(fi, nfi) {
		t.Fatalf("FileInfo not equal after healing")
	}

	// Test 4: checks if HealObject returns an error when xl.meta is not found
	// in more than read quorum number of disks, to create a corrupted situation.
	for i := 0; i <= nfi.Erasure.DataBlocks; i++ {
		erasureDisks[i].Delete(context.Background(), bucket, pathJoin(object, xlStorageFormatFile), false)
	}

	// Try healing now, expect to receive errFileNotFound.
	_, err = objLayer.HealObject(ctx, bucket, object, "", madmin.HealOpts{DryRun: false, Remove: true, ScanMode: madmin.HealDeepScan})
	if err != nil {
		if _, ok := err.(ObjectNotFound); !ok {
			t.Errorf("Expect %v but received %v", ObjectNotFound{Bucket: bucket, Object: object}, err)
		}
	}

	// since majority of xl.meta's are not available, object should be successfully deleted.
	_, err = objLayer.GetObjectInfo(ctx, bucket, object, ObjectOptions{})
	if _, ok := err.(ObjectNotFound); !ok {
		t.Errorf("Expect %v but received %v", ObjectNotFound{Bucket: bucket, Object: object}, err)
	}
}

// Tests healing of object.
func TestHealObjectErasure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nDisks := 16
	fsDirs, err := getRandomDisks(nDisks)
	if err != nil {
		t.Fatal(err)
	}

	defer removeRoots(fsDirs)

	// Everything is fine, should return nil
	obj, _, err := initObjectLayer(ctx, mustGetPoolEndpoints(fsDirs...))
	if err != nil {
		t.Fatal(err)
	}

	bucket := "bucket"
	object := "object"
	data := bytes.Repeat([]byte("a"), 5*1024*1024)
	var opts ObjectOptions

	err = obj.MakeBucketWithLocation(ctx, bucket, BucketOptions{})
	if err != nil {
		t.Fatalf("Failed to make a bucket - %v", err)
	}

	// Create an object with multiple parts uploaded in decreasing
	// part number.
	uploadID, err := obj.NewMultipartUpload(ctx, bucket, object, opts)
	if err != nil {
		t.Fatalf("Failed to create a multipart upload - %v", err)
	}

	var uploadedParts []CompletePart
	for _, partID := range []int{2, 1} {
		pInfo, err1 := obj.PutObjectPart(ctx, bucket, object, uploadID, partID, mustGetPutObjReader(t, bytes.NewReader(data), int64(len(data)), "", ""), opts)
		if err1 != nil {
			t.Fatalf("Failed to upload a part - %v", err1)
		}
		uploadedParts = append(uploadedParts, CompletePart{
			PartNumber: pInfo.PartNumber,
			ETag:       pInfo.ETag,
		})
	}

	// Remove the object backend files from the first disk.
	z := obj.(*erasureServerPools)
	er := z.serverPools[0].sets[0]
	firstDisk := er.getDisks()[0]

	_, err = obj.CompleteMultipartUpload(ctx, bucket, object, uploadID, uploadedParts, ObjectOptions{})
	if err != nil {
		t.Fatalf("Failed to complete multipart upload - %v", err)
	}

	err = firstDisk.Delete(context.Background(), bucket, pathJoin(object, xlStorageFormatFile), false)
	if err != nil {
		t.Fatalf("Failed to delete a file - %v", err)
	}

	_, err = obj.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealNormalScan})
	if err != nil {
		t.Fatalf("Failed to heal object - %v", err)
	}

	if err = firstDisk.CheckFile(context.Background(), bucket, object); err != nil {
		t.Errorf("Expected er.meta file to be present but stat failed - %v", err)
	}

	erasureDisks := er.getDisks()
	z.serverPools[0].erasureDisksMu.Lock()
	er.getDisks = func() []StorageAPI {
		// Nil more than half the disks, to remove write quorum.
		for i := 0; i <= len(erasureDisks)/2; i++ {
			erasureDisks[i] = nil
		}
		return erasureDisks
	}
	z.serverPools[0].erasureDisksMu.Unlock()

	// Try healing now, expect to receive errDiskNotFound.
	_, err = obj.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealDeepScan})
	// since majority of er.meta's are not available, object quorum can't be read properly and error will be errErasureReadQuorum
	if _, ok := err.(InsufficientReadQuorum); !ok {
		t.Errorf("Expected %v but received %v", InsufficientReadQuorum{}, err)
	}
}

// Tests healing of empty directories
func TestHealEmptyDirectoryErasure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nDisks := 16
	fsDirs, err := getRandomDisks(nDisks)
	if err != nil {
		t.Fatal(err)
	}
	defer removeRoots(fsDirs)

	// Everything is fine, should return nil
	obj, _, err := initObjectLayer(ctx, mustGetPoolEndpoints(fsDirs...))
	if err != nil {
		t.Fatal(err)
	}

	bucket := "bucket"
	object := "empty-dir/"
	var opts ObjectOptions

	err = obj.MakeBucketWithLocation(ctx, bucket, BucketOptions{})
	if err != nil {
		t.Fatalf("Failed to make a bucket - %v", err)
	}

	// Upload an empty directory
	_, err = obj.PutObject(ctx, bucket, object, mustGetPutObjReader(t,
		bytes.NewReader([]byte{}), 0, "", ""), opts)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the object backend files from the first disk.
	z := obj.(*erasureServerPools)
	er := z.serverPools[0].sets[0]
	firstDisk := er.getDisks()[0]
	err = firstDisk.DeleteVol(context.Background(), pathJoin(bucket, encodeDirObject(object)), true)
	if err != nil {
		t.Fatalf("Failed to delete a file - %v", err)
	}

	// Heal the object
	hr, err := obj.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealNormalScan})
	if err != nil {
		t.Fatalf("Failed to heal object - %v", err)
	}

	// Check if the empty directory is restored in the first disk
	_, err = firstDisk.StatVol(context.Background(), pathJoin(bucket, encodeDirObject(object)))
	if err != nil {
		t.Fatalf("Expected object to be present but stat failed - %v", err)
	}

	// Check the state of the object in the first disk (should be missing)
	if hr.Before.Drives[0].State != madmin.DriveStateMissing {
		t.Fatalf("Unexpected drive state: %v", hr.Before.Drives[0].State)
	}

	// Check the state of all other disks (should be ok)
	for i, h := range append(hr.Before.Drives[1:], hr.After.Drives...) {
		if h.State != madmin.DriveStateOk {
			t.Fatalf("Unexpected drive state (%d): %v", i+1, h.State)
		}
	}

	// Heal the same object again
	hr, err = obj.HealObject(ctx, bucket, object, "", madmin.HealOpts{ScanMode: madmin.HealNormalScan})
	if err != nil {
		t.Fatalf("Failed to heal object - %v", err)
	}

	// Check that Before & After states are all okay
	for i, h := range append(hr.Before.Drives, hr.After.Drives...) {
		if h.State != madmin.DriveStateOk {
			t.Fatalf("Unexpected drive state (%d): %v", i+1, h.State)
		}
	}
}
