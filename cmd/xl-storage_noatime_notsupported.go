//go:build windows || darwin || freebsd
// +build windows darwin freebsd

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
	"os"
)

var (
	// No special option for reads on windows
	readMode = os.O_RDONLY

	// Write with sync no buffering only used only for `xl.meta` writes
	writeMode = os.O_SYNC
)
