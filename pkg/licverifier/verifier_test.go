// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
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

package licverifier

import (
	"fmt"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// at fixes the jwt.TimeFunc at t and calls f in that context.
func at(t time.Time, f func()) {
	jwt.TimeFunc = func() time.Time { return t }
	f()
	jwt.TimeFunc = time.Now
}

func areEqLicenseInfo(a, b LicenseInfo) bool {
	if a.Email == b.Email && a.Organization == b.Organization && a.AccountID == b.AccountID && a.Plan == b.Plan && a.StorageCapacity == b.StorageCapacity && a.ExpiresAt.Equal(b.ExpiresAt) {
		return true
	}
	return false
}

// TestLicenseVerify tests the license key verification process with a valid and
// an invalid key.
func TestLicenseVerify(t *testing.T) {
	pemBytes := []byte(`-----BEGIN PUBLIC KEY-----
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEbo+e1wpBY4tBq9AONKww3Kq7m6QP/TBQ
mr/cKCUyBL7rcAvg0zNq1vcSrUSGlAmY3SEDCu3GOKnjG/U4E7+p957ocWSV+mQU
9NKlTdQFGF3+aO6jbQ4hX/S5qPyF+a3z
-----END PUBLIC KEY-----`)
	lv, err := NewLicenseVerifier(pemBytes)
	if err != nil {
		t.Fatalf("Failed to create license verifier: %s", err)
	}
	testCases := []struct {
		lic             string
		expectedLicInfo LicenseInfo
		shouldPass      bool
	}{{"", LicenseInfo{}, false},
		{"eyJhbGciOiJFUzM4NCIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJrYW5hZ2FyYWorYzFAbWluaW8uaW8iLCJjYXAiOjUwLCJvcmciOiJHcmluZ290dHMgSW5jLiIsImV4cCI6MS42NDE0NDYxNjkwMDExOTg4OTRlOSwicGxhbiI6IlNUQU5EQVJEIiwiaXNzIjoic3VibmV0QG1pbi5pbyIsImFpZCI6MSwiaWF0IjoxLjYwOTkxMDE2OTAwMTE5ODg5NGU5fQ.EhTL2xwMHnUoLQF4UR-5bjUCja3whseLU5mb9XEj7PvAae6HEIDCOMEF8Hhh20DN_v_LRE283j2ZlA5zulcXSZXS0CLcrKqbVy6QLvZfvvLuerOjJI-NBa9dSJWJ0WoN", LicenseInfo{
			Email:           "kanagaraj+c1@minio.io",
			Organization:    "Gringotts Inc.",
			AccountID:       1,
			StorageCapacity: 50,
			Plan:            "STANDARD",
			ExpiresAt:       time.Date(2022, time.January, 6, 5, 16, 9, 0, time.UTC),
		}, true},
	}

	for i, tc := range testCases {
		// Fixing the jwt.TimeFunc at 2021-01-06 05:16:09 +0000 UTC to
		// ensure that the license JWT doesn't expire ever.
		at(time.Unix(int64(1609910169), 0), func() {
			licInfo, err := lv.Verify(tc.lic)
			if err != nil && tc.shouldPass {
				t.Fatalf("%d: Expected license to pass verification but failed with %s", i+1, err)
			}
			if err == nil {
				if !tc.shouldPass {
					t.Fatalf("%d: Expected license to fail verification but passed", i+1)
				}
				if !areEqLicenseInfo(tc.expectedLicInfo, licInfo) {
					t.Fatalf("%d: Expected license info %v but got %v", i+1, tc.expectedLicInfo, licInfo)
				}
			}
		})
	}
}

// Example creates a LicenseVerifier using the ECDSA public key in pemBytes. It
// uses the Verify method of the LicenseVerifier to verify and extract the
// claims present in the license key.
func Example() {
	pemBytes := []byte(`-----BEGIN PUBLIC KEY-----
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEbo+e1wpBY4tBq9AONKww3Kq7m6QP/TBQ
mr/cKCUyBL7rcAvg0zNq1vcSrUSGlAmY3SEDCu3GOKnjG/U4E7+p957ocWSV+mQU
9NKlTdQFGF3+aO6jbQ4hX/S5qPyF+a3z
-----END PUBLIC KEY-----`)

	lv, err := NewLicenseVerifier(pemBytes)
	if err != nil {
		fmt.Println("Failed to create license verifier", err)
	}

	licenseKey := "eyJhbGciOiJFUzM4NCIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJrYW5hZ2FyYWorYzFAbWluaW8uaW8iLCJjYXAiOjUwLCJvcmciOiJHcmluZ290dHMgSW5jLiIsImV4cCI6MS42NDE0NDYxNjkwMDExOTg4OTRlOSwicGxhbiI6IlNUQU5EQVJEIiwiaXNzIjoic3VibmV0QG1pbi5pbyIsImFpZCI6MSwiaWF0IjoxLjYwOTkxMDE2OTAwMTE5ODg5NGU5fQ.EhTL2xwMHnUoLQF4UR-5bjUCja3whseLU5mb9XEj7PvAae6HEIDCOMEF8Hhh20DN_v_LRE283j2ZlA5zulcXSZXS0CLcrKqbVy6QLvZfvvLuerOjJI-NBa9dSJWJ0WoN"
	licInfo, err := lv.Verify(licenseKey)
	if err != nil {
		fmt.Println("Failed to verify license key", err)
	}

	fmt.Println("License metadata", licInfo)
}
