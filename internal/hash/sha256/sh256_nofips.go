// Copyright (c) 2015-2022 MinIO, Inc.
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

//go:build !fips
// +build !fips

package sha256

import (
	"hash"

	nofipssha256 "github.com/minio/sha256-simd"
)

// New returns a new hash.Hash computing the SHA256 checksum.
// The SHA256 implementation is not FIPS 140-2 compliant.
func New() hash.Hash { return nofipssha256.New() }

// Sum256 returns the SHA256 checksum of the data.
func Sum256(data []byte) [nofipssha256.Size]byte { return nofipssha256.Sum256(data) }
