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

package versioning

import (
	"fmt"
)

// Error is the generic type for any error happening during tag
// parsing.
type Error struct {
	err error
}

// Errorf - formats according to a format specifier and returns
// the string as a value that satisfies error of type tagging.Error
func Errorf(format string, a ...interface{}) error {
	return Error{err: fmt.Errorf(format, a...)}
}

// Unwrap the internal error.
func (e Error) Unwrap() error { return e.err }

// Error 'error' compatible method.
func (e Error) Error() string {
	if e.err == nil {
		return "versioning: cause <nil>"
	}
	return e.err.Error()
}
