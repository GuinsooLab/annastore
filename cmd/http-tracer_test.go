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
	"testing"
)

// Test redactLDAPPwd()
func TestRedactLDAPPwd(t *testing.T) {
	testCases := []struct {
		query         string
		expectedQuery string
	}{
		{"", ""},
		{
			"?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername&LDAPPassword=can+youreadthis%3F&Version=2011-06-15",
			"?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername&LDAPPassword=*REDACTED*&Version=2011-06-15",
		},
		{
			"LDAPPassword=can+youreadthis%3F&Version=2011-06-15&?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername",
			"LDAPPassword=*REDACTED*&Version=2011-06-15&?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername",
		},
		{
			"?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername&Version=2011-06-15&LDAPPassword=can+youreadthis%3F",
			"?Action=AssumeRoleWithLDAPIdentity&LDAPUsername=myusername&Version=2011-06-15&LDAPPassword=*REDACTED*",
		},
		{
			"?x=y&a=b",
			"?x=y&a=b",
		},
	}
	for i, test := range testCases {
		gotQuery := redactLDAPPwd(test.query)
		if gotQuery != test.expectedQuery {
			t.Fatalf("test %d: expected %s got %s", i+1, test.expectedQuery, gotQuery)
		}
	}
}
