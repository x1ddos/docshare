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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
)

const (
	usage          = "usage: docshare [-n] (-a|-r email-addr) doc-id [doc-id...]"
	maxConcurrency = 10
)

var (
	notifyFlag = flag.Bool("n", false, "notify shared-with parties via email")
	addUser    = flag.String("a", "", "email addr of user to add")
	remUser    = flag.String("r", "", "email addr of user to remove")
)

type permission struct {
	Role         string `json:"role"`
	Type         string `json:"type"`
	EmailAddress string `json:"emailAddress"`
}

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
		notifyParam := fmt.Sprintf("sendNotificationEmail=%v", *notifyFlag)
		for _, id := range flag.Args()[0:] {
			ch <- struct{}{}
			wg.Add(1)
			go func(id string, body []byte) {
				defer func() {
					<-ch
					wg.Done()
				}()
				url := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/permissions?%s", id, notifyParam)
				resp, err := client.Post(url, "application/json", bytes.NewReader(body))
				if err != nil {
					log.Printf("error posting to %s for doc %s: %v (%v)", url, id, err, resp.StatusCode)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					log.Printf("error posting to %s for doc %s: %v", url, id, resp.StatusCode)
				}
			}(id, body)
		}
	}
	if *remUser != "" {
		for _, id := range flag.Args()[0:] {
			ch <- struct{}{}
			wg.Add(1)
			go func(docID string) {
				defer func() {
					<-ch
					wg.Done()
				}()
				url := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/permissions?fields=permissions(emailAddress%%2Cid)", docID)
				resp, err := client.Get(url)
				if err != nil {
					log.Printf("error getting perms via %s for doc %s: %v (%v)", url, docID, err, resp.StatusCode)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					log.Printf("error getting perms via %s for doc %s: %v (%v)", url, docID, err, resp.StatusCode)
					return
				}
				var p struct {
					Permissions []*struct {
						ID           string `json:"id"`
						EmailAddress string `json:"emailAddress"`
					}
				}
				if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
					log.Printf("error parsing resp body from %s for doc %s", url, docID)
					return
				}
				var permID string
				for _, v := range p.Permissions {
					if v.EmailAddress == *remUser {
						permID = v.ID
						break
					}
				}
				if permID == "" {
					log.Printf("specified email addr (%s) not shared with doc %s", *remUser, docID)
					return
				}
				url = fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/permissions/%s", docID, permID)
				req, err := http.NewRequest("DELETE", url, nil)
				if err != nil {
					log.Printf("error building delete req %s for doc %s: %v", url, docID, err)
					return
				}
				resp, err = client.Do(req)
				if err != nil {
					log.Printf("error deleting perm %s for doc %s: %v (%v)", url, docID, err, resp.StatusCode)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 && resp.StatusCode != 204 {
					log.Printf("error deleting perm %s for doc %s: %v", url, docID, resp.StatusCode)
				}
			}(id)
		}
	}
	wg.Wait()
}
