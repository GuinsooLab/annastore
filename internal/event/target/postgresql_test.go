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

package target

import (
	"database/sql"
	"testing"
)

// TestPostgreSQLRegistration checks if postgres driver
// is registered and fails otherwise.
func TestPostgreSQLRegistration(t *testing.T) {
	var found bool
	for _, drv := range sql.Drivers() {
		if drv == "postgres" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("postgres driver not registered")
	}
}
