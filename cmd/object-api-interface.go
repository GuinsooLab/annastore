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
	"context"
	"io"
	"net/http"
	"time"

	"github.com/minio/madmin-go"
	"github.com/minio/minio-go/v7/pkg/encrypt"
	"github.com/minio/minio-go/v7/pkg/tags"
	"github.com/minio/pkg/bucket/policy"

	"github.com/GuinsooLab/annastore/internal/bucket/replication"
	xioutil "github.com/GuinsooLab/annastore/internal/ioutil"
)

// CheckPreconditionFn returns true if precondition check failed.
type CheckPreconditionFn func(o ObjectInfo) bool

// EvalMetadataFn validates input objInfo and returns an updated metadata
type EvalMetadataFn func(o ObjectInfo) error

// GetObjectInfoFn is the signature of GetObjectInfo function.
type GetObjectInfoFn func(ctx context.Context, bucket, object string, opts ObjectOptions) (ObjectInfo, error)

// ObjectOptions represents object options for ObjectLayer object operations
type ObjectOptions struct {
	ServerSideEncryption encrypt.ServerSide
	VersionSuspended     bool      // indicates if the bucket was previously versioned but is currently suspended.
	Versioned            bool      // indicates if the bucket is versioned
	VersionID            string    // Specifies the versionID which needs to be overwritten or read
	MTime                time.Time // Is only set in POST/PUT operations
	Expires              time.Time // Is only used in POST/PUT operations

	DeleteMarker      bool                // Is only set in DELETE operations for delete marker replication
	UserDefined       map[string]string   // only set in case of POST/PUT operations
	PartNumber        int                 // only useful in case of GetObject/HeadObject
	CheckPrecondFn    CheckPreconditionFn // only set during GetObject/HeadObject/CopyObjectPart preconditional valuation
	EvalMetadataFn    EvalMetadataFn      // only set for retention settings, meant to be used only when updating metadata in-place.
	DeleteReplication ReplicationState    // Represents internal replication state needed for Delete replication
	Transition        TransitionOptions
	Expiration        ExpirationOptions

	NoDecryption                        bool      // indicates if the stream must be decrypted.
	PreserveETag                        string    // preserves this etag during a PUT call.
	NoLock                              bool      // indicates to lower layers if the caller is expecting to hold locks.
	ProxyRequest                        bool      // only set for GET/HEAD in active-active replication scenario
	ProxyHeaderSet                      bool      // only set for GET/HEAD in active-active replication scenario
	ReplicationRequest                  bool      // true only if replication request
	ReplicationSourceTaggingTimestamp   time.Time // set if MinIOSourceTaggingTimestamp received
	ReplicationSourceLegalholdTimestamp time.Time // set if MinIOSourceObjectLegalholdTimestamp received
	ReplicationSourceRetentionTimestamp time.Time // set if MinIOSourceObjectRetentionTimestamp received
	DeletePrefix                        bool      // set true to enforce a prefix deletion, only application for DeleteObject API,

	Speedtest bool // object call specifically meant for SpeedTest code, set to 'true' when invoked by SpeedtestHandler.

	// Use the maximum parity (N/2), used when saving server configuration files
	MaxParity bool

	// SkipDecommissioned set to 'true' if the call requires skipping the pool being decommissioned.
	// mainly set for certain WRITE operations.
	SkipDecommissioned bool

	WalkAscending   bool                     // return Walk results in ascending order of versions
	PrefixEnabledFn func(prefix string) bool // function which returns true if versioning is enabled on prefix

	// IndexCB will return any index created but the compression.
	// Object must have been read at this point.
	IndexCB func() []byte
}

// ExpirationOptions represents object options for object expiration at objectLayer.
type ExpirationOptions struct {
	Expire bool
}

// TransitionOptions represents object options for transition ObjectLayer operation
type TransitionOptions struct {
	Status         string
	Tier           string
	ETag           string
	RestoreRequest *RestoreObjectRequest
	RestoreExpiry  time.Time
	ExpireRestored bool
}

// MakeBucketOptions represents bucket options for ObjectLayer bucket operations
type MakeBucketOptions struct {
	Location          string
	LockEnabled       bool
	VersioningEnabled bool
	ForceCreate       bool      // Create buckets even if they are already created.
	CreatedAt         time.Time // only for site replication
}

// DeleteBucketOptions provides options for DeleteBucket calls.
type DeleteBucketOptions struct {
	Force      bool             // Force deletion
	NoRecreate bool             // Do not recreate on delete failures
	SRDeleteOp SRBucketDeleteOp // only when site replication is enabled
}

// BucketOptions provides options for ListBuckets and GetBucketInfo call.
type BucketOptions struct {
	Deleted bool // true only when site replication is enabled
}

// SetReplicaStatus sets replica status and timestamp for delete operations in ObjectOptions
func (o *ObjectOptions) SetReplicaStatus(st replication.StatusType) {
	o.DeleteReplication.ReplicaStatus = st
	o.DeleteReplication.ReplicaTimeStamp = UTCNow()
}

// DeleteMarkerReplicationStatus - returns replication status of delete marker from DeleteReplication state in ObjectOptions
func (o *ObjectOptions) DeleteMarkerReplicationStatus() replication.StatusType {
	return o.DeleteReplication.CompositeReplicationStatus()
}

// VersionPurgeStatus - returns version purge status from DeleteReplication state in ObjectOptions
func (o *ObjectOptions) VersionPurgeStatus() VersionPurgeStatusType {
	return o.DeleteReplication.CompositeVersionPurgeStatus()
}

// SetDeleteReplicationState sets the delete replication options.
func (o *ObjectOptions) SetDeleteReplicationState(dsc ReplicateDecision, vID string) {
	o.DeleteReplication = ReplicationState{
		ReplicateDecisionStr: dsc.String(),
	}
	switch {
	case o.VersionID == "":
		o.DeleteReplication.ReplicationStatusInternal = dsc.PendingStatus()
		o.DeleteReplication.Targets = replicationStatusesMap(o.DeleteReplication.ReplicationStatusInternal)
	default:
		o.DeleteReplication.VersionPurgeStatusInternal = dsc.PendingStatus()
		o.DeleteReplication.PurgeTargets = versionPurgeStatusesMap(o.DeleteReplication.VersionPurgeStatusInternal)
	}
}

// PutReplicationState gets ReplicationState for PUT operation from ObjectOptions
func (o *ObjectOptions) PutReplicationState() (r ReplicationState) {
	rstatus, ok := o.UserDefined[ReservedMetadataPrefixLower+ReplicationStatus]
	if !ok {
		return
	}
	r.ReplicationStatusInternal = rstatus
	r.Targets = replicationStatusesMap(rstatus)
	return
}

// LockType represents required locking for ObjectLayer operations
type LockType int

const (
	noLock LockType = iota
	readLock
	writeLock
)

// BackendMetrics - represents bytes served from backend
type BackendMetrics struct {
	bytesReceived uint64
	bytesSent     uint64
	requestStats  RequestStats
}

// ObjectLayer implements primitives for object API layer.
type ObjectLayer interface {
	// Locking operations on object.
	NewNSLock(bucket string, objects ...string) RWLocker

	// Storage operations.
	Shutdown(context.Context) error
	NSScanner(ctx context.Context, bf *bloomFilter, updates chan<- DataUsageInfo, wantCycle uint32, scanMode madmin.HealScanMode) error
	BackendInfo() madmin.BackendInfo
	StorageInfo(ctx context.Context) (StorageInfo, []error)
	LocalStorageInfo(ctx context.Context) (StorageInfo, []error)

	// Bucket operations.
	MakeBucketWithLocation(ctx context.Context, bucket string, opts MakeBucketOptions) error
	GetBucketInfo(ctx context.Context, bucket string, opts BucketOptions) (bucketInfo BucketInfo, err error)
	ListBuckets(ctx context.Context, opts BucketOptions) (buckets []BucketInfo, err error)
	DeleteBucket(ctx context.Context, bucket string, opts DeleteBucketOptions) error
	ListObjects(ctx context.Context, bucket, prefix, marker, delimiter string, maxKeys int) (result ListObjectsInfo, err error)
	ListObjectsV2(ctx context.Context, bucket, prefix, continuationToken, delimiter string, maxKeys int, fetchOwner bool, startAfter string) (result ListObjectsV2Info, err error)
	ListObjectVersions(ctx context.Context, bucket, prefix, marker, versionMarker, delimiter string, maxKeys int) (result ListObjectVersionsInfo, err error)
	// Walk lists all objects including versions, delete markers.
	Walk(ctx context.Context, bucket, prefix string, results chan<- ObjectInfo, opts ObjectOptions) error

	// Object operations.

	// GetObjectNInfo returns a GetObjectReader that satisfies the
	// ReadCloser interface. The Close method unlocks the object
	// after reading, so it must always be called after usage.
	//
	// IMPORTANTLY, when implementations return err != nil, this
	// function MUST NOT return a non-nil ReadCloser.
	GetObjectNInfo(ctx context.Context, bucket, object string, rs *HTTPRangeSpec, h http.Header, lockType LockType, opts ObjectOptions) (reader *GetObjectReader, err error)
	GetObjectInfo(ctx context.Context, bucket, object string, opts ObjectOptions) (objInfo ObjectInfo, err error)
	PutObject(ctx context.Context, bucket, object string, data *PutObjReader, opts ObjectOptions) (objInfo ObjectInfo, err error)
	CopyObject(ctx context.Context, srcBucket, srcObject, destBucket, destObject string, srcInfo ObjectInfo, srcOpts, dstOpts ObjectOptions) (objInfo ObjectInfo, err error)
	DeleteObject(ctx context.Context, bucket, object string, opts ObjectOptions) (ObjectInfo, error)
	DeleteObjects(ctx context.Context, bucket string, objects []ObjectToDelete, opts ObjectOptions) ([]DeletedObject, []error)
	TransitionObject(ctx context.Context, bucket, object string, opts ObjectOptions) error
	RestoreTransitionedObject(ctx context.Context, bucket, object string, opts ObjectOptions) error

	// Multipart operations.
	ListMultipartUploads(ctx context.Context, bucket, prefix, keyMarker, uploadIDMarker, delimiter string, maxUploads int) (result ListMultipartsInfo, err error)
	NewMultipartUpload(ctx context.Context, bucket, object string, opts ObjectOptions) (uploadID string, err error)
	CopyObjectPart(ctx context.Context, srcBucket, srcObject, destBucket, destObject string, uploadID string, partID int,
		startOffset int64, length int64, srcInfo ObjectInfo, srcOpts, dstOpts ObjectOptions) (info PartInfo, err error)
	PutObjectPart(ctx context.Context, bucket, object, uploadID string, partID int, data *PutObjReader, opts ObjectOptions) (info PartInfo, err error)
	GetMultipartInfo(ctx context.Context, bucket, object, uploadID string, opts ObjectOptions) (info MultipartInfo, err error)
	ListObjectParts(ctx context.Context, bucket, object, uploadID string, partNumberMarker int, maxParts int, opts ObjectOptions) (result ListPartsInfo, err error)
	AbortMultipartUpload(ctx context.Context, bucket, object, uploadID string, opts ObjectOptions) error
	CompleteMultipartUpload(ctx context.Context, bucket, object, uploadID string, uploadedParts []CompletePart, opts ObjectOptions) (objInfo ObjectInfo, err error)

	// Policy operations
	SetBucketPolicy(context.Context, string, *policy.Policy) error
	GetBucketPolicy(context.Context, string) (*policy.Policy, error)
	DeleteBucketPolicy(context.Context, string) error

	// Supported operations check
	IsNotificationSupported() bool
	IsListenSupported() bool
	IsEncryptionSupported() bool
	IsTaggingSupported() bool
	IsCompressionSupported() bool
	SetDriveCounts() []int // list of erasure stripe size for each pool in order.

	// Healing operations.
	HealFormat(ctx context.Context, dryRun bool) (madmin.HealResultItem, error)
	HealBucket(ctx context.Context, bucket string, opts madmin.HealOpts) (madmin.HealResultItem, error)
	HealObject(ctx context.Context, bucket, object, versionID string, opts madmin.HealOpts) (madmin.HealResultItem, error)
	HealObjects(ctx context.Context, bucket, prefix string, opts madmin.HealOpts, fn HealObjectFn) error

	// Backend related metrics
	GetMetrics(ctx context.Context) (*BackendMetrics, error)

	// Returns health of the backend
	Health(ctx context.Context, opts HealthOptions) HealthResult
	ReadHealth(ctx context.Context) bool

	// Metadata operations
	PutObjectMetadata(context.Context, string, string, ObjectOptions) (ObjectInfo, error)

	// ObjectTagging operations
	PutObjectTags(context.Context, string, string, string, ObjectOptions) (ObjectInfo, error)
	GetObjectTags(context.Context, string, string, ObjectOptions) (*tags.Tags, error)
	DeleteObjectTags(context.Context, string, string, ObjectOptions) (ObjectInfo, error)
}

// GetObject - TODO(aead): This function just acts as an adapter for GetObject tests and benchmarks
// since the GetObject method of the ObjectLayer interface has been removed. Once, the
// tests are adjusted to use GetObjectNInfo this function can be removed.
func GetObject(ctx context.Context, api ObjectLayer, bucket, object string, startOffset int64, length int64, writer io.Writer, etag string, opts ObjectOptions) (err error) {
	var header http.Header
	if etag != "" {
		header.Set("ETag", etag)
	}
	Range := &HTTPRangeSpec{Start: startOffset, End: startOffset + length}

	reader, err := api.GetObjectNInfo(ctx, bucket, object, Range, header, readLock, opts)
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = xioutil.Copy(writer, reader)
	return err
}
