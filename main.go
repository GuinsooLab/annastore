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

package main // import "github.com/GuinsooLab/annastore"

import (
	"os"

	// MUST be first import.
	_ "github.com/GuinsooLab/annastore/internal/init"

	store "github.com/GuinsooLab/annastore/cmd"

	// Import gateway
	_ "github.com/GuinsooLab/annastore/cmd/gateway"
)

func main() {
	store.Main(os.Args)
}
