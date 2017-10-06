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

// Package testutil implements test utilities.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"streaming_hdp/previews"
)

// TestEnvironment holds the HDP proxy with a transport configured to
// communicate through the HDP proxy. It also has an origin server
// that is used for sending responses from the origin.
type TestEnvironment struct {
	// The HD Preview proxy.
	HDPProxy *httptest.Server

	// The HD Preview proxy handler.
	ProxyHandler http.Handler

	// The transport configured to talk through the HD Preview proxy.
	Transport *http.Transport

	// The HTTP server for the origin server.
	OriginServer *httptest.Server
}

// NewTestEnvironment constructs a new TestEnvironment that holds a HDPProxy,
// a transport, and a origin server. The components are connected as follows:
//	- Transport requests resources via HDPProxy.
//	- The originServer responses to a request from the proxy.
func NewTestEnvironment(previewsHandler previews.Handler, originHandler http.Handler) (*TestEnvironment, error) {
	hdpProxyServer := httptest.NewServer(previewsHandler)
	proxyURL, err := url.Parse(hdpProxyServer.URL)
	originServer := httptest.NewServer(originHandler)
	if err != nil {
		previewsHandler.Close()
		return nil, err
	}
	outTransport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
	}
	return &TestEnvironment{hdpProxyServer, http.Handler(previewsHandler),
		outTransport, originServer}, nil
}
