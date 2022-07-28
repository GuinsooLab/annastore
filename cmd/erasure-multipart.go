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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/readahead"
	"github.com/minio/minio-go/v7/pkg/set"
	xhttp "github.com/minio/minio/internal/http"
	"github.com/minio/minio/internal/logger"
	"github.com/minio/minio/internal/sync/errgroup"
	"github.com/minio/pkg/mimedb"
)

func (er erasureObjects) getUploadIDDir(bucket, object, uploadID string) string {
	return pathJoin(er.getMultipartSHADir(bucket, object), uploadID)
}

func (er erasureObjects) getMultipartSHADir(bucket, object string) string {
	return getSHA256Hash([]byte(pathJoin(bucket, object)))
}

// checkUploadIDExists - verify if a given uploadID exists and is valid.
func (er erasureObjects) checkUploadIDExists(ctx context.Context, bucket, object, uploadID string) (err error) {
	defer func() {
		if errors.Is(err, errFileNotFound) || errors.Is(err, errVolumeNotFound) {
			err = errUploadIDNotFound
		}
	}()

	disks := er.getDisks()

	// Read metadata associated with the object from all disks.
	metaArr, errs := readAllFileInfo(ctx, disks, minioMetaMultipartBucket, er.getUploadIDDir(bucket, object, uploadID), "", false)

	readQuorum, _, err := objectQuorumFromMeta(ctx, metaArr, errs, er.defaultParityCount)
	if err != nil {
		return err
	}

	if reducedErr := reduceReadQuorumErrs(ctx, errs, objectOpIgnoredErrs, readQuorum); reducedErr != nil {
		return reducedErr
	}

	// List all online disks.
	_, modTime := listOnlineDisks(disks, metaArr, errs)

	// Pick latest valid metadata.
	_, err = pickValidFileInfo(ctx, metaArr, modTime, readQuorum)
	return err
}

// Removes part.meta given by partName belonging to a mulitpart upload from minioMetaBucket
func (er erasureObjects) removePartMeta(bucket, object, uploadID, dataDir string, partNumber int) {
	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)
	curpartPath := pathJoin(uploadIDPath, dataDir, fmt.Sprintf("part.%d", partNumber))
	storageDisks := er.getDisks()

	g := errgroup.WithNErrs(len(storageDisks))
	for index, disk := range storageDisks {
		if disk == nil {
			continue
		}
		index := index
		g.Go(func() error {
			_ = storageDisks[index].Delete(context.TODO(), minioMetaMultipartBucket, curpartPath+".meta", DeleteOptions{
				Recursive: false,
				Force:     false,
			})

			return nil
		}, index)
	}
	g.Wait()
}

// Removes part given by partName belonging to a mulitpart upload from minioMetaBucket
func (er erasureObjects) removeObjectPart(bucket, object, uploadID, dataDir string, partNumber int) {
	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)
	curpartPath := pathJoin(uploadIDPath, dataDir, fmt.Sprintf("part.%d", partNumber))
	storageDisks := er.getDisks()

	g := errgroup.WithNErrs(len(storageDisks))
	for index, disk := range storageDisks {
		if disk == nil {
			continue
		}
		index := index
		g.Go(func() error {
			// Ignoring failure to remove parts that weren't present in CompleteMultipartUpload
			// requests. xl.meta is the authoritative source of truth on which parts constitute
			// the object. The presence of parts that don't belong in the object doesn't affect correctness.
			_ = storageDisks[index].Delete(context.TODO(), minioMetaMultipartBucket, curpartPath, DeleteOptions{
				Recursive: false,
				Force:     false,
			})
			_ = storageDisks[index].Delete(context.TODO(), minioMetaMultipartBucket, curpartPath+".meta", DeleteOptions{
				Recursive: false,
				Force:     false,
			})

			return nil
		}, index)
	}
	g.Wait()
}

// Clean-up the old multipart uploads. Should be run in a Go routine.
func (er erasureObjects) cleanupStaleUploads(ctx context.Context, expiry time.Duration) {
	// run multiple cleanup's local to this server.
	var wg sync.WaitGroup
	for _, disk := range er.getLoadBalancedLocalDisks() {
		if disk != nil {
			wg.Add(1)
			go func(disk StorageAPI) {
				defer wg.Done()
				er.cleanupStaleUploadsOnDisk(ctx, disk, expiry)
			}(disk)
		}
	}
	wg.Wait()
}

func (er erasureObjects) renameAll(ctx context.Context, bucket, prefix string) {
	var wg sync.WaitGroup
	for _, disk := range er.getDisks() {
		if disk == nil {
			continue
		}
		wg.Add(1)
		go func(disk StorageAPI) {
			defer wg.Done()
			disk.RenameFile(ctx, bucket, prefix, minioMetaTmpDeletedBucket, mustGetUUID())
		}(disk)
	}
	wg.Wait()
}

func (er erasureObjects) deleteAll(ctx context.Context, bucket, prefix string) {
	var wg sync.WaitGroup
	for _, disk := range er.getDisks() {
		if disk == nil {
			continue
		}
		wg.Add(1)
		go func(disk StorageAPI) {
			defer wg.Done()
			disk.Delete(ctx, bucket, prefix, DeleteOptions{
				Recursive: true,
				Force:     false,
			})
		}(disk)
	}
	wg.Wait()
}

// Remove the old multipart uploads on the given disk.
func (er erasureObjects) cleanupStaleUploadsOnDisk(ctx context.Context, disk StorageAPI, expiry time.Duration) {
	now := time.Now()
	diskPath := disk.Endpoint().Path

	readDirFn(pathJoin(diskPath, minioMetaMultipartBucket), func(shaDir string, typ os.FileMode) error {
		return readDirFn(pathJoin(diskPath, minioMetaMultipartBucket, shaDir), func(uploadIDDir string, typ os.FileMode) error {
			uploadIDPath := pathJoin(shaDir, uploadIDDir)
			fi, err := disk.ReadVersion(ctx, minioMetaMultipartBucket, uploadIDPath, "", false)
			if err != nil {
				return nil
			}
			wait := er.deletedCleanupSleeper.Timer(ctx)
			if now.Sub(fi.ModTime) > expiry {
				er.renameAll(ctx, minioMetaMultipartBucket, uploadIDPath)
			}
			wait()
			return nil
		})
	})

	readDirFn(pathJoin(diskPath, minioMetaTmpBucket), func(tmpDir string, typ os.FileMode) error {
		if tmpDir == ".trash/" { // do not remove .trash/ here, it has its own routines
			return nil
		}
		vi, err := disk.StatVol(ctx, pathJoin(minioMetaTmpBucket, tmpDir))
		if err != nil {
			return nil
		}
		wait := er.deletedCleanupSleeper.Timer(ctx)
		if now.Sub(vi.Created) > expiry {
			er.deleteAll(ctx, minioMetaTmpBucket, tmpDir)
		}
		wait()
		return nil
	})
}

// ListMultipartUploads - lists all the pending multipart
// uploads for a particular object in a bucket.
//
// Implements minimal S3 compatible ListMultipartUploads API. We do
// not support prefix based listing, this is a deliberate attempt
// towards simplification of multipart APIs.
// The resulting ListMultipartsInfo structure is unmarshalled directly as XML.
func (er erasureObjects) ListMultipartUploads(ctx context.Context, bucket, object, keyMarker, uploadIDMarker, delimiter string, maxUploads int) (result ListMultipartsInfo, err error) {
	auditObjectErasureSet(ctx, object, &er)

	result.MaxUploads = maxUploads
	result.KeyMarker = keyMarker
	result.Prefix = object
	result.Delimiter = delimiter

	var uploadIDs []string
	var disk StorageAPI
	for _, disk = range er.getLoadBalancedDisks(true) {
		uploadIDs, err = disk.ListDir(ctx, minioMetaMultipartBucket, er.getMultipartSHADir(bucket, object), -1)
		if err != nil {
			if errors.Is(err, errDiskNotFound) {
				continue
			}
			if errors.Is(err, errFileNotFound) || errors.Is(err, errVolumeNotFound) {
				return result, nil
			}
			logger.LogIf(ctx, err)
			return result, toObjectErr(err, bucket, object)
		}
		break
	}

	for i := range uploadIDs {
		uploadIDs[i] = strings.TrimSuffix(uploadIDs[i], SlashSeparator)
	}

	// S3 spec says uploadIDs should be sorted based on initiated time, we need
	// to read the metadata entry.
	var uploads []MultipartInfo

	populatedUploadIds := set.NewStringSet()

	for _, uploadID := range uploadIDs {
		if populatedUploadIds.Contains(uploadID) {
			continue
		}
		fi, err := disk.ReadVersion(ctx, minioMetaMultipartBucket, pathJoin(er.getUploadIDDir(bucket, object, uploadID)), "", false)
		if err != nil {
			if !IsErrIgnored(err, errFileNotFound, errDiskNotFound) {
				logger.LogIf(ctx, err)
			}
			// Ignore this invalid upload-id since we are listing here
			continue
		}
		populatedUploadIds.Add(uploadID)
		uploads = append(uploads, MultipartInfo{
			Object:    object,
			UploadID:  uploadID,
			Initiated: fi.ModTime,
		})
	}

	sort.Slice(uploads, func(i int, j int) bool {
		return uploads[i].Initiated.Before(uploads[j].Initiated)
	})

	uploadIndex := 0
	if uploadIDMarker != "" {
		for uploadIndex < len(uploads) {
			if uploads[uploadIndex].UploadID != uploadIDMarker {
				uploadIndex++
				continue
			}
			if uploads[uploadIndex].UploadID == uploadIDMarker {
				uploadIndex++
				break
			}
			uploadIndex++
		}
	}
	for uploadIndex < len(uploads) {
		result.Uploads = append(result.Uploads, uploads[uploadIndex])
		result.NextUploadIDMarker = uploads[uploadIndex].UploadID
		uploadIndex++
		if len(result.Uploads) == maxUploads {
			break
		}
	}

	result.IsTruncated = uploadIndex < len(uploads)

	if !result.IsTruncated {
		result.NextKeyMarker = ""
		result.NextUploadIDMarker = ""
	}

	return result, nil
}

// newMultipartUpload - wrapper for initializing a new multipart
// request; returns a unique upload id.
//
// Internally this function creates 'uploads.json' associated for the
// incoming object at
// '.minio.sys/multipart/bucket/object/uploads.json' on all the
// disks. `uploads.json` carries metadata regarding on-going multipart
// operation(s) on the object.
func (er erasureObjects) newMultipartUpload(ctx context.Context, bucket string, object string, opts ObjectOptions) (string, error) {
	userDefined := cloneMSS(opts.UserDefined)

	onlineDisks := er.getDisks()
	parityDrives := globalStorageClass.GetParityForSC(userDefined[xhttp.AmzStorageClass])
	if parityDrives < 0 {
		parityDrives = er.defaultParityCount
	}

	parityOrig := parityDrives
	for _, disk := range onlineDisks {
		if parityDrives >= len(onlineDisks)/2 {
			parityDrives = len(onlineDisks) / 2
			break
		}
		if disk == nil {
			parityDrives++
			continue
		}
		di, err := disk.DiskInfo(ctx)
		if err != nil || di.ID == "" {
			parityDrives++
		}
	}
	if parityOrig != parityDrives {
		userDefined[minIOErasureUpgraded] = strconv.Itoa(parityOrig) + "->" + strconv.Itoa(parityDrives)
	}

	dataDrives := len(onlineDisks) - parityDrives

	// we now know the number of blocks this object needs for data and parity.
	// establish the writeQuorum using this data
	writeQuorum := dataDrives
	if dataDrives == parityDrives {
		writeQuorum++
	}

	// Initialize parts metadata
	partsMetadata := make([]FileInfo, len(onlineDisks))

	fi := newFileInfo(pathJoin(bucket, object), dataDrives, parityDrives)
	fi.VersionID = opts.VersionID
	if opts.Versioned && fi.VersionID == "" {
		fi.VersionID = mustGetUUID()
	}
	fi.DataDir = mustGetUUID()

	// Initialize erasure metadata.
	for index := range partsMetadata {
		partsMetadata[index] = fi
	}

	// Guess content-type from the extension if possible.
	if userDefined["content-type"] == "" {
		userDefined["content-type"] = mimedb.TypeByExtension(path.Ext(object))
	}

	modTime := opts.MTime
	if opts.MTime.IsZero() {
		modTime = UTCNow()
	}

	onlineDisks, partsMetadata = shuffleDisksAndPartsMetadata(onlineDisks, partsMetadata, fi)

	// Fill all the necessary metadata.
	// Update `xl.meta` content on each disks.
	for index := range partsMetadata {
		partsMetadata[index].Fresh = true
		partsMetadata[index].ModTime = modTime
		partsMetadata[index].Metadata = userDefined
	}

	uploadID := mustGetUUID()
	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)

	// Write updated `xl.meta` to all disks.
	if _, err := writeUniqueFileInfo(ctx, onlineDisks, minioMetaMultipartBucket, uploadIDPath, partsMetadata, writeQuorum); err != nil {
		return "", toObjectErr(err, minioMetaMultipartBucket, uploadIDPath)
	}

	// Return success.
	return uploadID, nil
}

// NewMultipartUpload - initialize a new multipart upload, returns a
// unique id. The unique id returned here is of UUID form, for each
// subsequent request each UUID is unique.
//
// Implements S3 compatible initiate multipart API.
func (er erasureObjects) NewMultipartUpload(ctx context.Context, bucket, object string, opts ObjectOptions) (string, error) {
	auditObjectErasureSet(ctx, object, &er)

	return er.newMultipartUpload(ctx, bucket, object, opts)
}

// CopyObjectPart - reads incoming stream and internally erasure codes
// them. This call is similar to put object part operation but the source
// data is read from an existing object.
//
// Implements S3 compatible Upload Part Copy API.
func (er erasureObjects) CopyObjectPart(ctx context.Context, srcBucket, srcObject, dstBucket, dstObject, uploadID string, partID int, startOffset int64, length int64, srcInfo ObjectInfo, srcOpts, dstOpts ObjectOptions) (pi PartInfo, e error) {
	partInfo, err := er.PutObjectPart(ctx, dstBucket, dstObject, uploadID, partID, NewPutObjReader(srcInfo.Reader), dstOpts)
	if err != nil {
		return pi, toObjectErr(err, dstBucket, dstObject)
	}

	// Success.
	return partInfo, nil
}

func undoRenamePart(disks []StorageAPI, srcBucket, srcEntry, dstBucket, dstEntry string, errs []error) {
	// Undo rename object on disks where RenameFile succeeded.
	g := errgroup.WithNErrs(len(disks))
	for index, disk := range disks {
		if disk == nil {
			continue
		}
		index := index
		g.Go(func() error {
			if errs[index] == nil {
				_ = disks[index].RenameFile(context.TODO(), dstBucket, dstEntry, srcBucket, srcEntry)
			}
			return nil
		}, index)
	}
	g.Wait()
}

// renamePart - renames multipart part to its relevant location under uploadID.
func renamePart(ctx context.Context, disks []StorageAPI, srcBucket, srcEntry, dstBucket, dstEntry string, writeQuorum int) ([]StorageAPI, error) {
	g := errgroup.WithNErrs(len(disks))

	// Rename file on all underlying storage disks.
	for index := range disks {
		index := index
		g.Go(func() error {
			if disks[index] == nil {
				return errDiskNotFound
			}
			return disks[index].RenameFile(ctx, srcBucket, srcEntry, dstBucket, dstEntry)
		}, index)
	}

	// Wait for all renames to finish.
	errs := g.Wait()

	// We can safely allow RenameFile errors up to len(er.getDisks()) - writeQuorum
	// otherwise return failure. Cleanup successful renames.
	err := reduceWriteQuorumErrs(ctx, errs, objectOpIgnoredErrs, writeQuorum)
	if err == errErasureWriteQuorum {
		// Undo all the partial rename operations.
		undoRenamePart(disks, srcBucket, srcEntry, dstBucket, dstEntry, errs)
	}

	return evalDisks(disks, errs), err
}

// writeAllDisks - writes 'b' to all provided disks.
// If write cannot reach quorum, the files will be deleted from all disks.
func writeAllDisks(ctx context.Context, disks []StorageAPI, dstBucket, dstEntry string, b []byte, writeQuorum int) ([]StorageAPI, error) {
	g := errgroup.WithNErrs(len(disks))

	// Write file to all underlying storage disks.
	for index := range disks {
		index := index
		g.Go(func() error {
			if disks[index] == nil {
				return errDiskNotFound
			}
			return disks[index].WriteAll(ctx, dstBucket, dstEntry, b)
		}, index)
	}

	// Wait for all renames to finish.
	errs := g.Wait()

	// We can safely allow RenameFile errors up to len(er.getDisks()) - writeQuorum
	// otherwise return failure. Cleanup successful renames.
	err := reduceWriteQuorumErrs(ctx, errs, objectOpIgnoredErrs, writeQuorum)
	if err == errErasureWriteQuorum {
		// Remove all written
		g := errgroup.WithNErrs(len(disks))
		for index := range disks {
			if disks[index] == nil || errs[index] != nil {
				continue
			}
			index := index
			g.Go(func() error {
				return disks[index].Delete(ctx, dstBucket, dstEntry, DeleteOptions{Force: true})
			}, index)
		}
		// Ignore these errors.
		g.WaitErr()
	}

	return evalDisks(disks, errs), err
}

// PutObjectPart - reads incoming stream and internally erasure codes
// them. This call is similar to single put operation but it is part
// of the multipart transaction.
//
// Implements S3 compatible Upload Part API.
func (er erasureObjects) PutObjectPart(ctx context.Context, bucket, object, uploadID string, partID int, r *PutObjReader, opts ObjectOptions) (pi PartInfo, err error) {
	auditObjectErasureSet(ctx, object, &er)

	// Write lock for this part ID.
	// Held throughout the operation.
	partIDLock := er.NewNSLock(bucket, pathJoin(object, uploadID, strconv.Itoa(partID)))
	plkctx, err := partIDLock.GetLock(ctx, globalOperationTimeout)
	if err != nil {
		return PartInfo{}, err
	}
	pctx := plkctx.Context()
	defer partIDLock.Unlock(plkctx.Cancel)

	// Read lock for upload id.
	// Only held while reading the upload metadata.
	uploadIDRLock := er.NewNSLock(bucket, pathJoin(object, uploadID))
	rlkctx, err := uploadIDRLock.GetRLock(ctx, globalOperationTimeout)
	if err != nil {
		return PartInfo{}, err
	}
	rctx := rlkctx.Context()
	defer uploadIDRLock.RUnlock(rlkctx.Cancel)

	data := r.Reader
	// Validate input data size and it can never be less than zero.
	if data.Size() < -1 {
		logger.LogIf(rctx, errInvalidArgument, logger.Application)
		return pi, toObjectErr(errInvalidArgument)
	}

	var partsMetadata []FileInfo
	var errs []error
	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)

	// Validates if upload ID exists.
	if err = er.checkUploadIDExists(rctx, bucket, object, uploadID); err != nil {
		return pi, toObjectErr(err, bucket, object, uploadID)
	}

	storageDisks := er.getDisks()

	// Read metadata associated with the object from all disks.
	partsMetadata, errs = readAllFileInfo(rctx, storageDisks, minioMetaMultipartBucket,
		uploadIDPath, "", false)

	// get Quorum for this object
	_, writeQuorum, err := objectQuorumFromMeta(pctx, partsMetadata, errs, er.defaultParityCount)
	if err != nil {
		return pi, toObjectErr(err, bucket, object)
	}

	reducedErr := reduceWriteQuorumErrs(pctx, errs, objectOpIgnoredErrs, writeQuorum)
	if reducedErr == errErasureWriteQuorum {
		return pi, toObjectErr(reducedErr, bucket, object)
	}

	// List all online disks.
	onlineDisks, modTime := listOnlineDisks(storageDisks, partsMetadata, errs)

	// Pick one from the first valid metadata.
	fi, err := pickValidFileInfo(pctx, partsMetadata, modTime, writeQuorum)
	if err != nil {
		return pi, err
	}

	onlineDisks = shuffleDisks(onlineDisks, fi.Erasure.Distribution)

	// Need a unique name for the part being written in minioMetaBucket to
	// accommodate concurrent PutObjectPart requests

	partSuffix := fmt.Sprintf("part.%d", partID)
	tmpPart := mustGetUUID()
	tmpPartPath := pathJoin(tmpPart, partSuffix)

	// Delete the temporary object part. If PutObjectPart succeeds there would be nothing to delete.
	var online int
	defer func() {
		if online != len(onlineDisks) {
			er.renameAll(context.Background(), minioMetaTmpBucket, tmpPart)
		}
	}()

	erasure, err := NewErasure(pctx, fi.Erasure.DataBlocks, fi.Erasure.ParityBlocks, fi.Erasure.BlockSize)
	if err != nil {
		return pi, toObjectErr(err, bucket, object)
	}

	// Fetch buffer for I/O, returns from the pool if not allocates a new one and returns.
	var buffer []byte
	switch size := data.Size(); {
	case size == 0:
		buffer = make([]byte, 1) // Allocate atleast a byte to reach EOF
	case size == -1:
		if size := data.ActualSize(); size > 0 && size < fi.Erasure.BlockSize {
			buffer = make([]byte, data.ActualSize()+256, data.ActualSize()*2+512)
		} else {
			buffer = er.bp.Get()
			defer er.bp.Put(buffer)
		}
	case size >= fi.Erasure.BlockSize:
		buffer = er.bp.Get()
		defer er.bp.Put(buffer)
	case size < fi.Erasure.BlockSize:
		// No need to allocate fully fi.Erasure.BlockSize buffer if the incoming data is smaller.
		buffer = make([]byte, size, 2*size+int64(fi.Erasure.ParityBlocks+fi.Erasure.DataBlocks-1))
	}

	if len(buffer) > int(fi.Erasure.BlockSize) {
		buffer = buffer[:fi.Erasure.BlockSize]
	}
	writers := make([]io.Writer, len(onlineDisks))
	for i, disk := range onlineDisks {
		if disk == nil {
			continue
		}
		writers[i] = newBitrotWriter(disk, minioMetaTmpBucket, tmpPartPath, erasure.ShardFileSize(data.Size()), DefaultBitrotAlgorithm, erasure.ShardSize())
	}

	toEncode := io.Reader(data)
	if data.Size() > bigFileThreshold {
		// Add input readahead.
		// We use 2 buffers, so we always have a full buffer of input.
		bufA := er.bp.Get()
		bufB := er.bp.Get()
		defer er.bp.Put(bufA)
		defer er.bp.Put(bufB)
		ra, err := readahead.NewReaderBuffer(data, [][]byte{bufA[:fi.Erasure.BlockSize], bufB[:fi.Erasure.BlockSize]})
		if err == nil {
			toEncode = ra
			defer ra.Close()
		}
	}

	n, err := erasure.Encode(pctx, toEncode, writers, buffer, writeQuorum)
	closeBitrotWriters(writers)
	if err != nil {
		return pi, toObjectErr(err, bucket, object)
	}

	// Should return IncompleteBody{} error when reader has fewer bytes
	// than specified in request header.
	if n < data.Size() {
		return pi, IncompleteBody{Bucket: bucket, Object: object}
	}

	for i := range writers {
		if writers[i] == nil {
			onlineDisks[i] = nil
		}
	}

	// Rename temporary part file to its final location.
	partPath := pathJoin(uploadIDPath, fi.DataDir, partSuffix)
	onlineDisks, err = renamePart(ctx, onlineDisks, minioMetaTmpBucket, tmpPartPath, minioMetaMultipartBucket, partPath, writeQuorum)
	if err != nil {
		return pi, toObjectErr(err, minioMetaMultipartBucket, partPath)
	}

	md5hex := r.MD5CurrentHexString()
	if opts.PreserveETag != "" {
		md5hex = opts.PreserveETag
	}

	var index []byte
	if opts.IndexCB != nil {
		index = opts.IndexCB()
	}

	part := ObjectPartInfo{
		Number:     partID,
		ETag:       md5hex,
		Size:       n,
		ActualSize: data.ActualSize(),
		ModTime:    UTCNow(),
		Index:      index,
	}

	partMsg, err := part.MarshalMsg(nil)
	if err != nil {
		return pi, toObjectErr(err, minioMetaMultipartBucket, partPath)
	}

	// Write part metadata to all disks.
	onlineDisks, err = writeAllDisks(ctx, onlineDisks, minioMetaMultipartBucket, partPath+".meta", partMsg, writeQuorum)
	if err != nil {
		return pi, toObjectErr(err, minioMetaMultipartBucket, partPath)
	}

	// Return success.
	return PartInfo{
		PartNumber:   part.Number,
		ETag:         part.ETag,
		LastModified: part.ModTime,
		Size:         part.Size,
		ActualSize:   part.ActualSize,
	}, nil
}

// GetMultipartInfo returns multipart metadata uploaded during newMultipartUpload, used
// by callers to verify object states
// - encrypted
// - compressed
// Does not contain currently uploaded parts by design.
func (er erasureObjects) GetMultipartInfo(ctx context.Context, bucket, object, uploadID string, opts ObjectOptions) (MultipartInfo, error) {
	auditObjectErasureSet(ctx, object, &er)

	result := MultipartInfo{
		Bucket:   bucket,
		Object:   object,
		UploadID: uploadID,
	}

	uploadIDLock := er.NewNSLock(bucket, pathJoin(object, uploadID))
	lkctx, err := uploadIDLock.GetRLock(ctx, globalOperationTimeout)
	if err != nil {
		return MultipartInfo{}, err
	}
	ctx = lkctx.Context()
	defer uploadIDLock.RUnlock(lkctx.Cancel)

	if err := er.checkUploadIDExists(ctx, bucket, object, uploadID); err != nil {
		return result, toObjectErr(err, bucket, object, uploadID)
	}

	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)

	storageDisks := er.getDisks()

	// Read metadata associated with the object from all disks.
	partsMetadata, errs := readAllFileInfo(ctx, storageDisks, minioMetaMultipartBucket, uploadIDPath, opts.VersionID, false)

	// get Quorum for this object
	readQuorum, _, err := objectQuorumFromMeta(ctx, partsMetadata, errs, er.defaultParityCount)
	if err != nil {
		return result, toObjectErr(err, minioMetaMultipartBucket, uploadIDPath)
	}

	reducedErr := reduceWriteQuorumErrs(ctx, errs, objectOpIgnoredErrs, readQuorum)
	if reducedErr == errErasureReadQuorum {
		return result, toObjectErr(reducedErr, minioMetaMultipartBucket, uploadIDPath)
	}

	_, modTime := listOnlineDisks(storageDisks, partsMetadata, errs)

	// Pick one from the first valid metadata.
	fi, err := pickValidFileInfo(ctx, partsMetadata, modTime, readQuorum)
	if err != nil {
		return result, err
	}

	result.UserDefined = cloneMSS(fi.Metadata)
	return result, nil
}

// ListObjectParts - lists all previously uploaded parts for a given
// object and uploadID.  Takes additional input of part-number-marker
// to indicate where the listing should begin from.
//
// Implements S3 compatible ListObjectParts API. The resulting
// ListPartsInfo structure is marshaled directly into XML and
// replied back to the client.
func (er erasureObjects) ListObjectParts(ctx context.Context, bucket, object, uploadID string, partNumberMarker, maxParts int, opts ObjectOptions) (result ListPartsInfo, err error) {
	auditObjectErasureSet(ctx, object, &er)

	uploadIDLock := er.NewNSLock(bucket, pathJoin(object, uploadID))
	lkctx, err := uploadIDLock.GetRLock(ctx, globalOperationTimeout)
	if err != nil {
		return ListPartsInfo{}, err
	}
	ctx = lkctx.Context()
	defer uploadIDLock.RUnlock(lkctx.Cancel)

	if err := er.checkUploadIDExists(ctx, bucket, object, uploadID); err != nil {
		return result, toObjectErr(err, bucket, object, uploadID)
	}

	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)

	storageDisks := er.getDisks()

	// Read metadata associated with the object from all disks.
	partsMetadata, errs := readAllFileInfo(ctx, storageDisks, minioMetaMultipartBucket, uploadIDPath, "", false)

	// get Quorum for this object
	_, writeQuorum, err := objectQuorumFromMeta(ctx, partsMetadata, errs, er.defaultParityCount)
	if err != nil {
		return result, toObjectErr(err, minioMetaMultipartBucket, uploadIDPath)
	}

	reducedErr := reduceWriteQuorumErrs(ctx, errs, objectOpIgnoredErrs, writeQuorum)
	if reducedErr == errErasureWriteQuorum {
		return result, toObjectErr(reducedErr, minioMetaMultipartBucket, uploadIDPath)
	}

	onlineDisks, modTime := listOnlineDisks(storageDisks, partsMetadata, errs)

	// Pick one from the first valid metadata.
	fi, err := pickValidFileInfo(ctx, partsMetadata, modTime, writeQuorum)
	if err != nil {
		return result, err
	}

	if maxParts == 0 {
		return result, nil
	}

	if partNumberMarker < 0 {
		partNumberMarker = 0
	}

	// Limit output to maxPartsList.
	if maxParts > maxPartsList-partNumberMarker {
		maxParts = maxPartsList - partNumberMarker
	}

	// Read Part info for all parts
	partPath := pathJoin(uploadIDPath, fi.DataDir) + "/"
	req := ReadMultipleReq{
		Bucket:     minioMetaMultipartBucket,
		Prefix:     partPath,
		MaxSize:    1 << 20, // Each part should realistically not be > 1MiB.
		MaxResults: maxParts + 1,
	}

	start := partNumberMarker + 1
	end := start + maxParts

	// Parts are 1 based, so index 0 is part one, etc.
	for i := start; i <= end; i++ {
		req.Files = append(req.Files, fmt.Sprintf("part.%d.meta", i))
	}

	partInfoFiles, err := readMultipleFiles(ctx, onlineDisks, req, writeQuorum)
	if err != nil {
		return result, err
	}

	// Populate the result stub.
	result.Bucket = bucket
	result.Object = object
	result.UploadID = uploadID
	result.MaxParts = maxParts
	result.PartNumberMarker = partNumberMarker
	result.UserDefined = cloneMSS(fi.Metadata)

	// For empty number of parts or maxParts as zero, return right here.
	if len(partInfoFiles) == 0 || maxParts == 0 {
		return result, nil
	}

	var partI ObjectPartInfo
	for i, part := range partInfoFiles {
		partN := i + partNumberMarker + 1
		if part.Error != "" || !part.Exists {
			continue
		}

		_, err := partI.UnmarshalMsg(part.Data)
		if err != nil {
			// Maybe crash or similar.
			logger.LogIf(ctx, err)
			continue
		}

		if partN != partI.Number {
			logger.LogIf(ctx, fmt.Errorf("part.%d.meta has incorrect corresponding part number: expected %d, got %d", i+1, i+1, partI.Number))
			continue
		}

		// Add the current part.
		fi.AddObjectPart(partI.Number, partI.ETag, partI.Size, partI.ActualSize, partI.ModTime, partI.Index)
	}

	// Only parts with higher part numbers will be listed.
	parts := fi.Parts
	result.Parts = make([]PartInfo, 0, len(parts))
	for _, part := range parts {
		result.Parts = append(result.Parts, PartInfo{
			PartNumber:   part.Number,
			ETag:         part.ETag,
			LastModified: part.ModTime,
			ActualSize:   part.ActualSize,
			Size:         part.Size,
		})
		if len(result.Parts) >= maxParts {
			break
		}
	}

	// If listed entries are more than maxParts, we set IsTruncated as true.
	if len(parts) > len(result.Parts) {
		result.IsTruncated = true
		// Make sure to fill next part number marker if IsTruncated is
		// true for subsequent listing.
		nextPartNumberMarker := result.Parts[len(result.Parts)-1].PartNumber
		result.NextPartNumberMarker = nextPartNumberMarker
	}
	return result, nil
}

// CompleteMultipartUpload - completes an ongoing multipart
// transaction after receiving all the parts indicated by the client.
// Returns an md5sum calculated by concatenating all the individual
// md5sums of all the parts.
//
// Implements S3 compatible Complete multipart API.
func (er erasureObjects) CompleteMultipartUpload(ctx context.Context, bucket string, object string, uploadID string, parts []CompletePart, opts ObjectOptions) (oi ObjectInfo, err error) {
	auditObjectErasureSet(ctx, object, &er)

	// Hold write locks to verify uploaded parts, also disallows any
	// parallel PutObjectPart() requests.
	uploadIDLock := er.NewNSLock(bucket, pathJoin(object, uploadID))
	wlkctx, err := uploadIDLock.GetLock(ctx, globalOperationTimeout)
	if err != nil {
		return oi, err
	}
	wctx := wlkctx.Context()
	defer uploadIDLock.Unlock(wlkctx.Cancel)

	if err = er.checkUploadIDExists(wctx, bucket, object, uploadID); err != nil {
		return oi, toObjectErr(err, bucket, object, uploadID)
	}

	uploadIDPath := er.getUploadIDDir(bucket, object, uploadID)

	storageDisks := er.getDisks()

	// Read metadata associated with the object from all disks.
	partsMetadata, errs := readAllFileInfo(wctx, storageDisks, minioMetaMultipartBucket, uploadIDPath, "", false)

	// get Quorum for this object
	_, writeQuorum, err := objectQuorumFromMeta(wctx, partsMetadata, errs, er.defaultParityCount)
	if err != nil {
		return oi, toObjectErr(err, bucket, object)
	}

	reducedErr := reduceWriteQuorumErrs(wctx, errs, objectOpIgnoredErrs, writeQuorum)
	if reducedErr == errErasureWriteQuorum {
		return oi, toObjectErr(reducedErr, bucket, object)
	}

	onlineDisks, modTime := listOnlineDisks(storageDisks, partsMetadata, errs)

	// Pick one from the first valid metadata.
	fi, err := pickValidFileInfo(wctx, partsMetadata, modTime, writeQuorum)
	if err != nil {
		return oi, err
	}

	// Read Part info for all parts
	partPath := pathJoin(uploadIDPath, fi.DataDir) + "/"
	req := ReadMultipleReq{
		Bucket:     minioMetaMultipartBucket,
		Prefix:     partPath,
		MaxSize:    1 << 20, // Each part should realistically not be > 1MiB.
		Files:      make([]string, 0, len(parts)),
		AbortOn404: true,
	}
	for _, part := range parts {
		req.Files = append(req.Files, fmt.Sprintf("part.%d.meta", part.PartNumber))
	}
	partInfoFiles, err := readMultipleFiles(ctx, onlineDisks, req, writeQuorum)
	if err != nil {
		return oi, err
	}
	if len(partInfoFiles) != len(parts) {
		// Should only happen through internal error
		err := fmt.Errorf("unexpected part result count: %d, want %d", len(partInfoFiles), len(parts))
		logger.LogIf(ctx, err)
		return oi, toObjectErr(err, bucket, object)
	}

	var partI ObjectPartInfo
	for i, part := range partInfoFiles {
		partID := parts[i].PartNumber
		if part.Error != "" || !part.Exists {
			return oi, InvalidPart{
				PartNumber: partID,
			}
		}

		_, err := partI.UnmarshalMsg(part.Data)
		if err != nil {
			// Maybe crash or similar.
			logger.LogIf(ctx, err)
			return oi, InvalidPart{
				PartNumber: partID,
			}
		}

		if partID != partI.Number {
			logger.LogIf(ctx, fmt.Errorf("part.%d.meta has incorrect corresponding part number: expected %d, got %d", partID, partID, partI.Number))
			return oi, InvalidPart{
				PartNumber: partID,
			}
		}

		// Add the current part.
		fi.AddObjectPart(partI.Number, partI.ETag, partI.Size, partI.ActualSize, partI.ModTime, partI.Index)
	}

	// Calculate full object size.
	var objectSize int64

	// Calculate consolidated actual size.
	var objectActualSize int64

	// Order online disks in accordance with distribution order.
	// Order parts metadata in accordance with distribution order.
	onlineDisks, partsMetadata = shuffleDisksAndPartsMetadataByIndex(onlineDisks, partsMetadata, fi)

	// Save current erasure metadata for validation.
	currentFI := fi

	// Allocate parts similar to incoming slice.
	fi.Parts = make([]ObjectPartInfo, len(parts))

	// Validate each part and then commit to disk.
	for i, part := range parts {
		partIdx := objectPartIndex(currentFI.Parts, part.PartNumber)
		// All parts should have same part number.
		if partIdx == -1 {
			invp := InvalidPart{
				PartNumber: part.PartNumber,
				GotETag:    part.ETag,
			}
			return oi, invp
		}

		// ensure that part ETag is canonicalized to strip off extraneous quotes
		part.ETag = canonicalizeETag(part.ETag)
		if currentFI.Parts[partIdx].ETag != part.ETag {
			invp := InvalidPart{
				PartNumber: part.PartNumber,
				ExpETag:    currentFI.Parts[partIdx].ETag,
				GotETag:    part.ETag,
			}
			return oi, invp
		}

		// All parts except the last part has to be at least 5MB.
		if (i < len(parts)-1) && !isMinAllowedPartSize(currentFI.Parts[partIdx].ActualSize) {
			return oi, PartTooSmall{
				PartNumber: part.PartNumber,
				PartSize:   currentFI.Parts[partIdx].ActualSize,
				PartETag:   part.ETag,
			}
		}

		// Save for total object size.
		objectSize += currentFI.Parts[partIdx].Size

		// Save the consolidated actual size.
		objectActualSize += currentFI.Parts[partIdx].ActualSize

		// Add incoming parts.
		fi.Parts[i] = ObjectPartInfo{
			Number:     part.PartNumber,
			Size:       currentFI.Parts[partIdx].Size,
			ActualSize: currentFI.Parts[partIdx].ActualSize,
			ModTime:    currentFI.Parts[partIdx].ModTime,
			Index:      currentFI.Parts[partIdx].Index,
		}
	}

	// Save the final object size and modtime.
	fi.Size = objectSize
	fi.ModTime = opts.MTime
	if opts.MTime.IsZero() {
		fi.ModTime = UTCNow()
	}

	// Save successfully calculated md5sum.
	fi.Metadata["etag"] = opts.UserDefined["etag"]
	if fi.Metadata["etag"] == "" {
		fi.Metadata["etag"] = getCompleteMultipartMD5(parts)
	}

	// Save the consolidated actual size.
	fi.Metadata[ReservedMetadataPrefix+"actual-size"] = strconv.FormatInt(objectActualSize, 10)

	// Update all erasure metadata, make sure to not modify fields like
	// checksum which are different on each disks.
	for index := range partsMetadata {
		if partsMetadata[index].IsValid() {
			partsMetadata[index].Size = fi.Size
			partsMetadata[index].ModTime = fi.ModTime
			partsMetadata[index].Metadata = fi.Metadata
			partsMetadata[index].Parts = fi.Parts
		}
	}

	// Hold namespace to complete the transaction
	lk := er.NewNSLock(bucket, object)
	lkctx, err := lk.GetLock(ctx, globalOperationTimeout)
	if err != nil {
		return oi, err
	}
	ctx = lkctx.Context()
	defer lk.Unlock(lkctx.Cancel)

	// Write final `xl.meta` at uploadID location
	onlineDisks, err = writeUniqueFileInfo(ctx, onlineDisks, minioMetaMultipartBucket, uploadIDPath, partsMetadata, writeQuorum)
	if err != nil {
		return oi, toObjectErr(err, minioMetaMultipartBucket, uploadIDPath)
	}

	// Remove parts that weren't present in CompleteMultipartUpload request.
	for _, curpart := range currentFI.Parts {
		if objectPartIndex(fi.Parts, curpart.Number) == -1 {
			// Delete the missing part files. e.g,
			// Request 1: NewMultipart
			// Request 2: PutObjectPart 1
			// Request 3: PutObjectPart 2
			// Request 4: CompleteMultipartUpload --part 2
			// N.B. 1st part is not present. This part should be removed from the storage.
			er.removeObjectPart(bucket, object, uploadID, fi.DataDir, curpart.Number)
		}
	}

	// Remove part.meta which is not needed anymore.
	for _, part := range fi.Parts {
		er.removePartMeta(bucket, object, uploadID, fi.DataDir, part.Number)
	}

	// Rename the multipart object to final location.
	if onlineDisks, err = renameData(ctx, onlineDisks, minioMetaMultipartBucket, uploadIDPath,
		partsMetadata, bucket, object, writeQuorum); err != nil {
		return oi, toObjectErr(err, bucket, object)
	}

	// Check if there is any offline disk and add it to the MRF list
	for _, disk := range onlineDisks {
		if disk != nil && disk.IsOnline() {
			continue
		}
		er.addPartial(bucket, object, fi.VersionID, fi.Size)
		break
	}

	for i := 0; i < len(onlineDisks); i++ {
		if onlineDisks[i] != nil && onlineDisks[i].IsOnline() {
			// Object info is the same in all disks, so we can pick
			// the first meta from online disk
			fi = partsMetadata[i]
			break
		}
	}

	// we are adding a new version to this object under the namespace lock, so this is the latest version.
	fi.IsLatest = true

	// Success, return object info.
	return fi.ToObjectInfo(bucket, object, opts.Versioned || opts.VersionSuspended), nil
}

// AbortMultipartUpload - aborts an ongoing multipart operation
// signified by the input uploadID. This is an atomic operation
// doesn't require clients to initiate multiple such requests.
//
// All parts are purged from all disks and reference to the uploadID
// would be removed from the system, rollback is not possible on this
// operation.
func (er erasureObjects) AbortMultipartUpload(ctx context.Context, bucket, object, uploadID string, opts ObjectOptions) (err error) {
	auditObjectErasureSet(ctx, object, &er)

	lk := er.NewNSLock(bucket, pathJoin(object, uploadID))
	lkctx, err := lk.GetLock(ctx, globalOperationTimeout)
	if err != nil {
		return err
	}
	ctx = lkctx.Context()
	defer lk.Unlock(lkctx.Cancel)

	// Validates if upload ID exists.
	if err := er.checkUploadIDExists(ctx, bucket, object, uploadID); err != nil {
		return toObjectErr(err, bucket, object, uploadID)
	}

	// Cleanup all uploaded parts.
	er.renameAll(ctx, minioMetaMultipartBucket, er.getUploadIDDir(bucket, object, uploadID))

	// Successfully purged.
	return nil
}
