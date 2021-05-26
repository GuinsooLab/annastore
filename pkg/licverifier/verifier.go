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

// Package licverifier implements a simple library to verify MinIO Subnet license keys.
package licverifier

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// LicenseVerifier needs an ECDSA public key in PEM format for initialization.
type LicenseVerifier struct {
	ecPubKey *ecdsa.PublicKey
}

// LicenseInfo holds customer metadata present in the license key.
type LicenseInfo struct {
	Email           string    // Email of the license key requestor
	Organization    string    // Subnet organization name
	AccountID       int64     // Subnet account id
	StorageCapacity int64     // Storage capacity used in TB
	Plan            string    // Subnet plan
	ExpiresAt       time.Time // Time of license expiry
}

// license key JSON field names
const (
	accountID    = "aid"
	sub          = "sub"
	expiresAt    = "exp"
	organization = "org"
	capacity     = "cap"
	plan         = "plan"
)

// NewLicenseVerifier returns an initialized license verifier with the given
// ECDSA public key in PEM format.
func NewLicenseVerifier(pemBytes []byte) (*LicenseVerifier, error) {
	pbKey, err := jwt.ParseECPublicKeyFromPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse public key: %s", err)
	}
	return &LicenseVerifier{
		ecPubKey: pbKey,
	}, nil
}

// toLicenseInfo extracts LicenseInfo from claims. It returns an error if any of
// the claim values are invalid.
func toLicenseInfo(claims jwt.MapClaims) (LicenseInfo, error) {
	accID, ok := claims[accountID].(float64)
	if !ok || ok && accID <= 0 {
		return LicenseInfo{}, errors.New("Invalid accountId in claims")
	}
	email, ok := claims[sub].(string)
	if !ok {
		return LicenseInfo{}, errors.New("Invalid email in claims")
	}
	expiryTS, ok := claims[expiresAt].(float64)
	if !ok {
		return LicenseInfo{}, errors.New("Invalid time of expiry in claims")
	}
	expiresAt := time.Unix(int64(expiryTS), 0)
	orgName, ok := claims[organization].(string)
	if !ok {
		return LicenseInfo{}, errors.New("Invalid organization in claims")
	}
	storageCap, ok := claims[capacity].(float64)
	if !ok {
		return LicenseInfo{}, errors.New("Invalid storage capacity in claims")
	}
	plan, ok := claims[plan].(string)
	if !ok {
		return LicenseInfo{}, errors.New("Invalid plan in claims")
	}
	return LicenseInfo{
		Email:           email,
		Organization:    orgName,
		AccountID:       int64(accID),
		StorageCapacity: int64(storageCap),
		Plan:            plan,
		ExpiresAt:       expiresAt,
	}, nil

}

// Verify verifies the license key and validates the claims present in it.
func (lv *LicenseVerifier) Verify(license string) (LicenseInfo, error) {
	token, err := jwt.ParseWithClaims(license, &jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		return lv.ecPubKey, nil
	})
	if err != nil {
		return LicenseInfo{}, fmt.Errorf("Failed to verify license: %s", err)
	}
	if claims, ok := token.Claims.(*jwt.MapClaims); ok && token.Valid {
		return toLicenseInfo(*claims)
	}
	return LicenseInfo{}, errors.New("Invalid claims found in license")
}
