// +build ignore

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

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio/pkg/auth"
)

// AssumedRoleUser - The identifiers for the temporary security credentials that
// the operation returns. Please also see https://docs.aws.amazon.com/goto/WebAPI/sts-2011-06-15/AssumedRoleUser
type AssumedRoleUser struct {
	Arn           string
	AssumedRoleID string `xml:"AssumeRoleId"`
	// contains filtered or unexported fields
}

// AssumeRoleWithWebIdentityResponse contains the result of successful AssumeRoleWithWebIdentity request.
type AssumeRoleWithWebIdentityResponse struct {
	XMLName          xml.Name          `xml:"https://sts.amazonaws.com/doc/2011-06-15/ AssumeRoleWithWebIdentityResponse" json:"-"`
	Result           WebIdentityResult `xml:"AssumeRoleWithWebIdentityResult"`
	ResponseMetadata struct {
		RequestID string `xml:"RequestId,omitempty"`
	} `xml:"ResponseMetadata,omitempty"`
}

// WebIdentityResult - Contains the response to a successful AssumeRoleWithWebIdentity
// request, including temporary credentials that can be used to make MinIO API requests.
type WebIdentityResult struct {
	AssumedRoleUser             AssumedRoleUser  `xml:",omitempty"`
	Audience                    string           `xml:",omitempty"`
	Credentials                 auth.Credentials `xml:",omitempty"`
	PackedPolicySize            int              `xml:",omitempty"`
	Provider                    string           `xml:",omitempty"`
	SubjectFromWebIdentityToken string           `xml:",omitempty"`
}

// Returns a base64 encoded random 32 byte string.
func randomState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

var (
	stsEndpoint    string
	configEndpoint string
	clientID       string
	clientSec      string
	clientScopes   string
	port           int
)

// DiscoveryDoc - parses the output from openid-configuration
// for example http://localhost:8080/auth/realms/minio/.well-known/openid-configuration
type DiscoveryDoc struct {
	Issuer                           string   `json:"issuer,omitempty"`
	AuthEndpoint                     string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                    string   `json:"token_endpoint,omitempty"`
	UserInfoEndpoint                 string   `json:"userinfo_endpoint,omitempty"`
	RevocationEndpoint               string   `json:"revocation_endpoint,omitempty"`
	JwksURI                          string   `json:"jwks_uri,omitempty"`
	ResponseTypesSupported           []string `json:"response_types_supported,omitempty"`
	SubjectTypesSupported            []string `json:"subject_types_supported,omitempty"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported,omitempty"`
	ScopesSupported                  []string `json:"scopes_supported,omitempty"`
	TokenEndpointAuthMethods         []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	ClaimsSupported                  []string `json:"claims_supported,omitempty"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported,omitempty"`
}

func parseDiscoveryDoc(ustr string) (DiscoveryDoc, error) {
	d := DiscoveryDoc{}
	req, err := http.NewRequest(http.MethodGet, ustr, nil)
	if err != nil {
		return d, err
	}
	clnt := http.Client{
		Transport: http.DefaultTransport,
	}
	resp, err := clnt.Do(req)
	if err != nil {
		return d, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return d, err
	}
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&d); err != nil {
		return d, err
	}
	return d, nil
}

func init() {
	flag.StringVar(&stsEndpoint, "sts-ep", "http://localhost:9000", "STS endpoint")
	flag.StringVar(&configEndpoint, "config-ep",
		"http://localhost:8080/auth/realms/minio/.well-known/openid-configuration",
		"OpenID discovery document endpoint")
	flag.StringVar(&clientID, "cid", "", "Client ID")
	flag.StringVar(&clientSec, "csec", "", "Client Secret")
	flag.StringVar(&clientScopes, "cscopes", "openid", "Client Scopes")
	flag.IntVar(&port, "port", 8080, "Port")
}

func main() {
	flag.Parse()
	if clientID == "" || clientSec == "" {
		flag.PrintDefaults()
		return
	}

	ddoc, err := parseDiscoveryDoc(configEndpoint)
	if err != nil {
		log.Println(fmt.Errorf("Failed to parse OIDC discovery document %s", err))
		fmt.Println(err)
		return
	}

	scopes := ddoc.ScopesSupported
	if clientScopes != "" {
		scopes = strings.Split(clientScopes, ",")
	}

	ctx := context.Background()

	config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSec,
		Endpoint: oauth2.Endpoint{
			AuthURL:  ddoc.AuthEndpoint,
			TokenURL: ddoc.TokenEndpoint,
		},
		RedirectURL: fmt.Sprintf("http://localhost:%d/oauth2/callback", port),
		Scopes:      scopes,
	}

	state := randomState()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		if r.RequestURI != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, config.AuthCodeURL(state), http.StatusFound)
	})

	http.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.RequestURI)
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state did not match", http.StatusBadRequest)
			return
		}

		getWebTokenExpiry := func() (*credentials.WebIdentityToken, error) {
			oauth2Token, err := config.Exchange(ctx, r.URL.Query().Get("code"))
			if err != nil {
				return nil, err
			}
			if !oauth2Token.Valid() {
				return nil, errors.New("invalid token")
			}

			return &credentials.WebIdentityToken{
				Token:  oauth2Token.Extra("id_token").(string),
				Expiry: int(oauth2Token.Expiry.Sub(time.Now().UTC()).Seconds()),
			}, nil
		}

		sts, err := credentials.NewSTSWebIdentity(stsEndpoint, getWebTokenExpiry)
		if err != nil {
			log.Println(fmt.Errorf("Could not get STS credentials: %s", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		opts := &minio.Options{
			Creds:        sts,
			BucketLookup: minio.BucketLookupAuto,
		}

		u, err := url.Parse(stsEndpoint)
		if err != nil {
			log.Println(fmt.Errorf("Failed to parse STS Endpoint: %s", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		clnt, err := minio.New(u.Host, opts)
		if err != nil {
			log.Println(fmt.Errorf("Error while initializing Minio client, %s", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		buckets, err := clnt.ListBuckets(r.Context())
		if err != nil {
			log.Println(fmt.Errorf("Error while listing buckets, %s", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		creds, _ := sts.Get()

		bucketNames := []string{}

		for _, bucket := range buckets {
			log.Println(fmt.Sprintf("Bucket discovered: %s", bucket.Name))
			bucketNames = append(bucketNames, bucket.Name)
		}
		response := make(map[string]interface{})
		response["credentials"] = creds
		response["buckets"] = bucketNames
		c, err := json.MarshalIndent(response, "", "\t")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(c)
	})

	address := fmt.Sprintf("localhost:%v", port)
	log.Printf("listening on http://%s/", address)
	log.Fatal(http.ListenAndServe(address, nil))
}
