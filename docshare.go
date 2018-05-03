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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	usage          = "usage: docshare [-n] (-a|-r email-addr) doc-id [doc-id...]"
	maxConcurrency = 10
	driveAPI       = "https://www.googleapis.com/drive/v3"
)

var (
	notifyFlag = flag.Bool("n", false, "notify shared-with parties via email")
	addUser    = flag.String("a", "", "email addr of user to add")
	remUser    = flag.String("r", "", "email addr of user to remove")
)

// errNotFound is an internal error indicating a missing resource.
var errNotFound = errors.New("resource not found")

// docshare - add and/or remove sharing permissions on one or more Google Docs.
func main() {
	flag.Parse()
	var wg sync.WaitGroup
	if flag.NArg() < 1 || (*addUser == "" && *remUser == "") {
		log.Fatal(usage)
	}
	ch := make(chan struct{}, maxConcurrency)
	client, err := driveClient()
	if err != nil {
		log.Fatalf("can't get drive client: %v", err)
	}
	if *addUser != "" {
		body, err := json.Marshal(&permission{
			Role:         "reader",
			Type:         "user",
			EmailAddress: *addUser,
		})
		if err != nil {
			log.Fatalf("error marshaling body: %v", err)
		}
		params := url.Values{
			"sendNotificationEmail": {fmt.Sprintf("%v", *notifyFlag)},
			"supportsTeamDrives":    {"true"},
		}
		for _, id := range flag.Args()[0:] {
			ch <- struct{}{}
			wg.Add(1)
			go func(id string, body []byte) {
				defer func() {
					<-ch
					wg.Done()
				}()
				url := fmt.Sprintf("%s/files/%s/permissions?%s", driveAPI, id, params.Encode())
				resp, err := client.Post(url, "application/json", bytes.NewReader(body))
				if err != nil {
					log.Printf("error posting to %s for doc %s: %v (%v)", url, id, err, resp.StatusCode)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					log.Printf("%s: %v", id, errorResponse(resp))
				}
			}(id, body)
		}
	}
	if *remUser != "" {
		for _, id := range flag.Args()[0:] {
			ch <- struct{}{}
			wg.Add(1)
			go func(docID string) {
				// TODO: figure out a better way to create context for each goroutine.
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer func() {
					cancel()
					<-ch
					wg.Done()
				}()
				p, err := fetchPermission(ctx, client, docID, *remUser, pageToken(""))
				if err == errNotFound {
					// Already removed. Nothing to do.
					return
				}
				if err != nil {
					log.Printf("%s: %v", docID, err)
					return
				}
				u := fmt.Sprintf("%s/files/%s/permissions/%s?supportsTeamDrives=true", driveAPI, docID, p.ID)
				req, err := http.NewRequest("DELETE", u, nil)
				if err != nil {
					log.Printf("error building delete req %s for doc %s: %v", u, docID, err)
					return
				}
				resp, err := client.Do(req.WithContext(ctx))
				if err != nil {
					log.Printf("error deleting perm %s for doc %s: %v (%v)", u, docID, err, resp.StatusCode)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 && resp.StatusCode != 204 {
					log.Printf("%s: %v", docID, errorResponse(resp))
				}
			}(id)
		}
	}
	wg.Wait()
}

type pageToken string

func (t pageToken) zero() bool {
	return t == ""
}

type permission struct {
	ID           string `json:"id,omitempty"`
	Role         string `json:"role"`
	Type         string `json:"type"`
	EmailAddress string `json:"emailAddress"`
}

func fetchPermission(ctx context.Context, client *http.Client, docID, email string, token pageToken) (*permission, error) {
	params := url.Values{
		"supportsTeamDrives": {"true"},
		"pageSize":           {"10"},
		"fields":             {"nextPageToken,permissions(id,emailAddress)"},
	}
	if !token.zero() {
		params.Set("pageToken", string(token))
	}
	r, err := http.NewRequest("GET", fmt.Sprintf("%s/files/%s/permissions?%s", driveAPI, docID, params.Encode()), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(r.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errorResponse(resp)
	}
	var p struct {
		NextPageToken string
		Permissions   []*permission
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	for _, v := range p.Permissions {
		if v.EmailAddress == email {
			if v.ID == "" {
				return nil, fmt.Errorf("no permission ID for %s", email)
			}
			return v, nil
		}
	}
	if len(p.Permissions) == 0 || p.NextPageToken == "" {
		return nil, errNotFound
	}
	return fetchPermission(ctx, client, docID, email, pageToken(p.NextPageToken))
}

func errorResponse(res *http.Response) error {
	var e struct {
		Error struct {
			Errors []struct{ Message string }
		}
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.New(res.Status)
	}
	err = json.Unmarshal(b, &e)
	if err != nil || len(e.Error.Errors) == 0 {
		return fmt.Errorf("%s: %s", res.Status, b)
	}
	var a []string
	for _, v := range e.Error.Errors {
		a = append(a, v.Message)
	}
	return errors.New(strings.Join(a, "; "))
}
