//go:build !windows
// +build !windows

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
	"errors"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/djherbis/atime"
)

// Return error if Atime is disabled on the O/S
func checkAtimeSupport(dir string) (err error) {
	file, err := ioutil.TempFile(dir, "prefix")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	finfo1, err := os.Stat(file.Name())
	if err != nil {
		return
	}
	// add a sleep to ensure atime change is detected
	time.Sleep(10 * time.Millisecond)

	if _, err = io.Copy(ioutil.Discard, file); err != nil {
		return
	}

	finfo2, err := os.Stat(file.Name())

	if atime.Get(finfo2).Equal(atime.Get(finfo1)) {
		return errors.New("Atime not supported")
	}
	return
}
