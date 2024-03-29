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

package cmd

// DO NOT EDIT THIS FILE DIRECTLY. These are build-time constants
// set through ‘buildscripts/gen-ldflags.go’.
var (
	// GOPATH - GOPATH value at the time of build.
	GOPATH = ""

	// GOROOT - GOROOT value at the time of build.
	GOROOT = ""

	// Version - version time.RFC3339.
	Version = "DEVELOPMENT.GOGET"

	// ReleaseTag - release tag in TAG.%Y-%m-%dT%H-%M-%SZ.
	ReleaseTag = "DEVELOPMENT.GOGET"

	// CommitID - latest commit id.
	CommitID = "DEVELOPMENT.GOGET"

	// ShortCommitID - first 12 characters from CommitID.
	ShortCommitID = "DEVELOPMENT.GOGET"

	// CopyrightYear - dynamic value of the copyright end year
	CopyrightYear = "0000"
)
