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

package devtools

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Launch Chrome.
func startChrome(t *testing.T, devToolsPort int) (*exec.Cmd, string) {
	userDir, err := ioutil.TempDir("/tmp/", "")
	if err != nil {
		t.Fatalf("Failed to create userDir: %v with error: %v\n", userDir, err)
	}

	args := []string{"--user-data-dir=" + userDir, fmt.Sprintf("--remote-debugging-port=%d", devToolsPort), "about:blank"}
	chromeCmd := exec.Command("google-chrome", args...)
	if err := chromeCmd.Start(); err != nil {
		t.Fatalf("Failed to start Chrome: %v\n", err)
	}
	return chromeCmd, userDir
}

// Tests that events are being propagated back via the provided channel.
func TestReceivingEventsViaChannels(t *testing.T) {
	originServer := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	port := 9222
	chrome, usrDir := startChrome(t, port)
	defer chrome.Process.Kill()
	defer os.RemoveAll(usrDir)

	// This sleep is necessary to allow time for Chrome to set up, before connecting to it via devtools.
	time.Sleep(5 * time.Second)

	// Connect to Chrome using the Chrome Devtools Protocol.
	connection, err := NewConnection(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatalf("Failed to connect to Chrome on port %d with error: %v\n", port, err)
	}
	defer connection.Close()

	// Enables Page events and methods.
	connection.InvokeMethod("Page.enable", Params{})

	// Registers a callback for when a page is finished loading.
	loaded := make(chan bool)
	connection.InvokeMethod("Page.navigate", Params{"url": originServer.URL})
	go func(connection *Connection) {
		pageLoaded := false
		for {
			event, err := connection.NextEvent()
			if err == io.EOF {
				// no more events to process.
				break
			}
			if pageLoaded {
				// Throw all events away because the page has loaded.
				continue
			}
			switch {
			case event.Method == "Page.loadEventFired":
				loaded <- true
				pageLoaded = true
			default:
				// Do nothing.
			}
		}
	}(connection)
	<-loaded // Wait until page loads.
}

func TestInvokeMethodAndGetReturn(t *testing.T) {
	port := 9222
	chrome, usrDir := startChrome(t, port)
	defer chrome.Process.Kill()
	defer os.RemoveAll(usrDir)

	// This sleep is necessary to allow time for Chrome to set up, before connecting to it via devtools.
	time.Sleep(5 * time.Second)

	// Connect to Chrome using the Chrome Devtools Protocol.
	connection, err := NewConnection(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatalf("Failed to connect to Chrome on port %d with error: %v\n", port, err)
	}
	defer connection.Close()

	t.Run("Method_not_exists", func(t *testing.T) {
		// Tests that when sending a random method that does not exists in Chrome
		// devtools should return an error.
		result := connection.InvokeMethodAndGetReturn("Random.MethodThatWillNeverExists", Params{})
		if result.Type != ResultError {
			t.Errorf("The result should be an error.\n")
		}
	})

	t.Run("get_grandchild", func(t *testing.T) {
		result := connection.InvokeMethodAndGetReturn("Runtime.evaluate", Params{
			"expression":    `a={x: 42}; a`,
			"returnByValue": true,
		})
		val, ok := result.Params.Int("result.value.x")
		if !ok || val != 42 {
			t.Logf("result=%v", result)
			t.Fatalf("Int('result.value.x'): val,ok = (%v, %v), want (%v, %v)", val, ok, 42, true)
		}
	})
}
