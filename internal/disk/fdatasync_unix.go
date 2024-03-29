//go:build freebsd || netbsd || openbsd || darwin
// +build freebsd netbsd openbsd darwin

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

package disk

import (
	"os"
	"syscall"
)

// Fdatasync is fsync on freebsd/darwin
func Fdatasync(f *os.File) error {
	return syscall.Fsync(int(f.Fd()))
}

// FadviseDontNeed is a no-op
func FadviseDontNeed(f *os.File) error {
	return nil
}
