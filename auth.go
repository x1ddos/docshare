// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// auth scopes needed by the program
	scopeDrive = "https://www.googleapis.com/auth/drive"

	// program credentials for installed apps
	googClient = "561645876975-huk89daoabn20et9r3bl1k25eqoppomo.apps.googleusercontent.com"
	googSecret = "Wqw-VK5faU-k7dZ_R72qXSw2"
)

var (
	// OAuth2 configs for OOB flow
	tokenConf = &oauth2.Config{
		ClientID:     googClient,
		ClientSecret: googSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{scopeDrive},
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	}
)

// driveClient returns an HTTP client which knows how to perform authenticated
// requests to Google Drive API.
func driveClient() (*http.Client, error) {
	ts, err := tokenSource()
	if err != nil {
		return nil, err
	}
	t := &oauth2.Transport{
		Source: ts,
		Base:   http.DefaultTransport,
	}
	return &http.Client{Transport: t}, nil
}

// tokenSource creates a new oauth2.TokenSource backed by tokenRefresher,
// using previously stored user credentials if available.
func tokenSource() (oauth2.TokenSource, error) {
	t, err := readToken()
	if err != nil {
		t, err = authorize(tokenConf)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to obtain access token")
	}
	cache := &cachedTokenSource{
		src:    tokenConf.TokenSource(context.Background(), t),
		config: tokenConf,
	}
	return oauth2.ReuseTokenSource(nil, cache), nil
}

// authorize performs user authorization flow, asking for permissions grant.
func authorize(conf *oauth2.Config) (*oauth2.Token, error) {
	aurl := conf.AuthCodeURL("unused", oauth2.AccessTypeOffline)
	fmt.Printf("Authorize me at following URL, please:\n\n%s\n\nCode: ", aurl)
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, err
	}
	return conf.Exchange(context.Background(), code)
}

// cachedTokenSource stores tokens returned from src on local disk.
// It is usually combined with oauth2.ReuseTokenSource.
type cachedTokenSource struct {
	src    oauth2.TokenSource
	config *oauth2.Config
}

func (c *cachedTokenSource) Token() (*oauth2.Token, error) {
	t, err := c.src.Token()
	if err != nil {
		t, err = authorize(c.config)
	}
	if err != nil {
		return nil, err
	}
	writeToken(t)
	return t, nil
}

// readToken deserializes token from local disk.
func readToken() (*oauth2.Token, error) {
	l, err := tokenLocation()
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadFile(l)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	return t, json.Unmarshal(b, t)
}

// writeToken serializes token tok to local disk.
func writeToken(tok *oauth2.Token) error {
	l, err := tokenLocation()
	if err != nil {
		return err
	}
	w, err := os.Create(l)
	if err != nil {
		return err
	}
	defer w.Close()
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// tokenLocation returns a local file path, suitable for storing user credentials.
func tokenLocation() (string, error) {
	d := homedir()
	if d == "" {
		log.Printf("WARNING: unable to identify user home dir")
	}
	d = path.Join(d, ".config", "docshare")
	if err := os.MkdirAll(d, 0700); err != nil {
		return "", err
	}
	return path.Join(d, "goog-cred.json"), nil
}

func homedir() string {
	if v := os.Getenv("HOME"); v != "" {
		return v
	}
	d, p := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH")
	if d != "" && p != "" {
		return d + p
	}
	return os.Getenv("USERPROFILE")
}
