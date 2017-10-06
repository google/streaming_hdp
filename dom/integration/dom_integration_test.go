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

package test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phayes/freeport"

	"streaming_hdp/chrome"
	"streaming_hdp/dom"
)

func TestGenerateDOM(t *testing.T) {
	// This spins up Chrome and have it navigate to a test server.
	tests := []struct {
		dom                string
		expectedNumUpdates int
		label              string
	}{
		{
			label: "Initial DOM",
			dom: `<html>
			<head><script src="foo_2.js"></script></head>
			<body><script src="foo.js"></script><div>bar</div></body>
			</html>`,
			expectedNumUpdates: 7, // 6 tags and 1 text node.
		},
	}
	for _, test := range tests {
		domModel := dom.NewDOMModel()
		t.Run(test.label, func(t *testing.T) {
			originServer := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					io.WriteString(w, test.dom)
				},
			))
			// Start and connect to an instance of Chrome.
			chromePort, err := freeport.GetFreePort()
			if err != nil {
				t.Fatalf("cannot pick a port.")
			}

			chromeInstance, err := chrome.New(chromePort, true)
			if err != nil {
				t.Fatalf("failed to create a Chrome instance")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := chromeInstance.Wait(ctx); err != nil {
				t.Fatalf("timed out connecting to Chrome")
			}

			err = chromeInstance.Connect()
			if err != nil {
				t.Fatalf("failed to connect Chrome")
			}
			defer chromeInstance.DisconnectAndTerminate()

			// (3) navigate to the page and the get the response.
			chromeInstance.NavigateToPage(originServer.URL)
			loaded := make(chan struct{})
			go func() {
				pageStabilized := false
				for {
					event, err := chromeInstance.NextEvent()
					if err == io.EOF {
						// no more events to process.
						break
					}
					if pageStabilized {
						// Throw all events away because the page has loaded.
						continue
					}
					switch {
					case event.Method == "Emulation.virtualTimeBudgetExpired":
						// Page stablized.
						pageStabilized = true
						close(loaded)
					}
				}
			}()
			<-loaded // Wait for the page to be loaded
			rootNode, err := chromeInstance.GetDOMInstance()
			if err != nil {
				t.Errorf("failed to get the DOM instance: %v", err)
			}

			result, err := domModel.GenerateInitialDOM(rootNode)
			if err != nil {
				t.Errorf("failed to generate initial dom: %v", err)

			}
			if len(result) != test.expectedNumUpdates {
				t.Errorf("expected: %v updates, but got %v with updates: %v", test.expectedNumUpdates, len(result), result)
			}
		})
	}
}
