//go:build linux
// +build linux

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

package mountinfo

// mountInfo - This represents a single line in /proc/mounts.
type mountInfo struct {
	Device  string
	Path    string
	FSType  string
	Options []string
	Freq    string
	Pass    string
}

func (m mountInfo) String() string {
	return m.Path
}

// mountInfos - This represents the entire /proc/mounts.
type mountInfos []mountInfo
