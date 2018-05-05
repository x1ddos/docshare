package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

// See https://developers.google.com/drive/v3/web/handle-errors
// for details about error and reason codes.

type apiError struct {
	Code    int
	Message string
	Errors  []struct {
		Domain   string
		Message  string
		Reason   string
		Location string
	}
}

func (e *apiError) Error() string {
	var a []string
	for _, v := range e.Errors {
		a = append(a, v.Message)
	}
	return strings.Join(a, "; ")
}

func errorResponse(res *http.Response) error {
	r := &io.LimitedReader{R: res.Body, N: 1 << 20} // 1Mb
	b, err := ioutil.ReadAll(r)
	// Worst case: we can't even read the response body.
	if err != nil {
		return errors.New(res.Status)
	}
	var e struct {
		Code    int
		Message string
		Error   struct {
			Errors []struct {
				Domain   string
				Message  string
				Reason   string
				Location string
			}
		}
	}
	err = json.Unmarshal(b, &e)
	// Use raw body if we can't parse it.
	if err != nil || len(e.Error.Errors) == 0 {
		n := len(b)
		if n > 1024 {
			n = 1024
		}
		return fmt.Errorf("%s: %s", res.Status, b[:n])
	}
	// Best case: structured error with Code and Reason.
	return &apiError{
		Code:    e.Code,
		Message: e.Message,
		Errors:  e.Error.Errors,
	}
}

func isRetriable(status int, err error) bool {
	if status >= 500 || status == http.StatusTooManyRequests {
		return true
	}
	ae, ok := err.(*apiError)
	if !ok {
		return false
	}
	for _, v := range ae.Errors {
		switch v.Reason {
		case "userRateLimitExceeded", "rateLimitExceeded", "sharingRateLimitExceeded":
			return true
		}
	}
	return false
}
