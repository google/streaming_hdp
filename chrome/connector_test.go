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

package chrome

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Note: None of the tests work on Forge. However, it works locally.
// To test this locally run: blaze test :chrome --test_strategy local

// Test just launching and terminating Chrome.
func TestLaunchAndTerminateChrome(t *testing.T) {
	t.Logf("Started TestLaunchAndTerminateChrome\n")
	chromeInstance, err := New(9222, true)
	if err != nil {
		t.Fatalf("Could not launch Chrome: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if chromeInstance.Wait(ctx) != nil {
		t.Fatalf("Error getting Chrome's process information: %v", err)
	}

	err = chromeInstance.Connect()
	if err != nil {
		t.Errorf("Failed to connect Chrome.")
	}

	chromeInstance.InitializeTimeout()

	chromeInstance.DisconnectAndTerminate()
	if _, err = os.Stat(chromeInstance.userDir); !os.IsNotExist(err) {
		t.Errorf("UserDir: %v should have been deleted upon termination.\n", chromeInstance.userDir)
	}
}

// Test timeout implementation when Chrome is idle for more than 5 seconds.
func TestTimeoutTriggered(t *testing.T) {
	chromeInstance, err := New(9223, true)
	if err != nil {
		t.Fatalf("Could not launch Chrome: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if chromeInstance.Wait(ctx) != nil {
		t.Fatalf("Error getting Chrome's process information: %v", err)
	}

	err = chromeInstance.Connect()
	if err != nil {
		t.Errorf("Failed to connect Chrome.")
	}

	originalTimeout := instanceTimeout
	instanceTimeout = time.Millisecond
	chromeInstance.InitializeTimeout()
	instanceTimeout = originalTimeout

	// This will implicitly test whether the timeout works. If it doesn't,
	// this will hang forever and the test will eventually timeout.
	chromeInstance.Command.Wait()
}

// Test navigate to page by just seeing that it finishes at one point and
// make sure that the events are being received properly.
func TestNavigateToPageAndDomainEvents(t *testing.T) {
	tests := []struct {
		label           string
		expectedRetCode int
	}{
		{"Valid site", http.StatusOK},
		{"Nonexisting site", http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			originServer := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					if r.UserAgent() != userAgentString {
						// Make sure that we successfully faked the UA.
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(test.expectedRetCode)
				},
			))
			chromeInstance, err := New(9222, true)
			if err != nil {
				t.Fatalf("Could not launch Chrome: %v", err)
			}
			t.Logf("Returned from New() with instance: %v\n", chromeInstance)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if chromeInstance.Wait(ctx) != nil {
				t.Fatalf("Error getting Chrome's process information: %v", err)
			}

			err = chromeInstance.Connect()
			if err != nil {
				t.Fatalf("Failed to connect Chrome.")
			}
			chromeInstance.InitializeTimeout()

			// Enable Network domain and setup callbacks.
			chromeInstance.EnableDomains("Network")
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
					case event.Method == "Network.responseReceived":
						params := event.Params
						response := params["response"].(map[string]interface{})
						if got, want := response["status"], float64(test.expectedRetCode); got != want {
							t.Errorf("Incorrect response status got: %v, want: %v\n", got, want)
						}
					case event.Method == "Emulation.virtualTimeBudgetExpired":
						// Page stablized.
						t.Logf("Page stabilized\n")
						pageStabilized = true
						close(loaded)
					}
				}
			}()
			<-loaded // Wait for the page to be loaded

			if err := chromeInstance.DisconnectAndTerminate(); err != nil {
				t.Errorf("chromeInstance.DisconnectAndTerminate: %v", err)
			}
			if _, err = os.Stat(chromeInstance.userDir); !os.IsNotExist(err) {
				t.Errorf("UserDir: %v should have been deleted upon termination.\n", chromeInstance.userDir)
			}
		})
	}
}
