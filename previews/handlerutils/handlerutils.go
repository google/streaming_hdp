// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package handlerutils implements shared functions among previews handlers.
package handlerutils

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/phayes/freeport"

	"streaming_hdp/chrome"
)

// IsEventHandler checks if the string, s, is a string representing an event handler or not.
func IsEventHandler(s string) bool {
	// Strip onload, onclick, etc. We are assuming that all attributes starting
	// with "on" that will be added in the future are event handlers.
	// Currently, there are no known attributes with prefix "on" that are not
	// event handlers. See
	// https://developer.mozilla.org/en-US/docs/Web/HTML/Attributes#Attribute_list
	// and https://www.w3.org/TR/2011/WD-html5-20110525/elements.html). If this
	// ever changes, we should switch to a whitelist of event handlers.
	return strings.HasPrefix(s, "on")
}

// IsDocument checks if the response is a document by using the MIME type.
func IsDocument(response *http.Response) bool {
	return strings.Contains(response.Header.Get("Content-Type"), "text/html")
}

// Passthrough writes the HTTP response back to the http.ResponseWrite.
func Passthrough(rw http.ResponseWriter, response *http.Response) {
	rw.WriteHeader(response.StatusCode)
	io.Copy(rw, response.Body)
}

// CreateChromeInstance creates and wait until Chrome has started
// to return the started instance.
func CreateChromeInstance(useFullChrome bool) (*chrome.Instance, error) {
	// Start and connect to an instance of Chrome.
	chromePort, err := freeport.GetFreePort()
	if err != nil {
		return nil, err
	}

	// We don't care about the ID of Chrome in this case.
	chromeInstance, err := chrome.New(chromePort, useFullChrome)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := chromeInstance.Wait(ctx); err != nil {
		return nil, err
	}
	return chromeInstance, nil
}
