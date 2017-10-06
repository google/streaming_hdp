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

package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"streaming_hdp/chrome"
	"streaming_hdp/dom/domjson"
)

func TestWSStreamingUpdates(t *testing.T) {
	tests := []struct {
		dom                string
		expectedNumUpdates int
		label              string
	}{
		{
			label: "General",
			dom: `<html>
			<head><script src="foo_2.js"></script></head>
			<body><script src="foo.js"></script><div>bar</div></body>
			</html>`,
			expectedNumUpdates: 8, // 7 + 1 #document node.
		},
		{
			label: "Inserted via JS",
			dom: `<html>
			<head><script src="foo.js"></script></head>
			<body>
			<script>
				var div = document.createElement('div');
				document.getElementsByTagName("body")[0].appendChild(div);
			</script>
			<div>bar</div></body>
			</html>`,
			expectedNumUpdates: 9, // 9 - 1 from removing text node of script.
		},
		{
			label: "Removed via JS",
			dom: `<html>
			<head><script src="foo.js"></script></head>
			<body>
			<div id="remove_me">bar</div>
			<script>
				document.getElementById("remove_me").remove();
			</script>
			</body>
			</html>`,
			expectedNumUpdates: 6, // 6 - 1 from removing text node of script.
		},
	}
	chromeInstanceManager := chrome.NewInstanceManager(true)
	streamHandler, err := New(chromeInstanceManager, true /* for verbosity. */)
	if err != nil {
		t.Fatalf("failed to create stream handler  %v", err)
	}
	http.Handle("/stream", streamHandler)
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			proxyServer := httptest.NewServer(http.DefaultServeMux)
			originServer := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					io.WriteString(w, test.dom)
				},
			))
			t.Logf("Getting new instance of Chrome")
			chromeID := chromeInstanceManager.GetNewInstance(originServer.URL)
			chromeInstance, err := chromeInstanceManager.GetInstance(chromeID)
			if err != nil {
				t.Fatalf("failed to get chrome instance: %v", err)
			}

			t.Logf("Waiting for Chrome to be ready: %v", chromeID)
			err = chromeInstance.WaitUntilChromeReady()
			if err != nil || !chromeInstance.ResetTimeout() { // The timer already expired.
				t.Fatalf("failed after waiting chrome to be ready: %v", err)
			}
			t.Logf("Got Chrome: %v", chromeID)

			// Subscribe to events.
			chromeInstance.EnableDomains("DOM")
			chromeInstance.NavigateToPage(originServer.URL)

			// (2) try to connect to websocket with the ID.
			streamURL := "http://" + proxyServer.Listener.Addr().String() + "/stream?id=" + strconv.Itoa(chromeID)

			resp, err := http.Get(streamURL)
			if err != nil {
				t.Errorf("failed to connect to stream: %v", err)
			}

			elementsCount := 0
			scriptNodes := make(map[string]bool)
			reader := bufio.NewReader(resp.Body)
			for {
				message, err := reader.ReadBytes([]byte(delim)[0])
				if err == io.EOF {
					break
				}
				updates := &domjson.DOMUpdates{}
				if err = json.Unmarshal(bytes.TrimSpace(message), updates); err != nil {
					t.Fatalf("json message in an incorrect format: %v, err: %v", string(bytes.TrimSpace(message)), err)
				}
				for _, update := range updates.Updates {
					if strings.ToLower(update.Node.ElementType) == "script" {
						if len(update.Node.Attributes) > 0 {
							t.Errorf("script %v still have attributes", update)
						}
						scriptNodes[update.Node.NodeID] = true
					} else {
						if _, ok := scriptNodes[update.Node.ParentNodeID]; ok {
							t.Errorf("script node should not have a child")
						}
					}
				}
				elementsCount += len(updates.Updates)
			}

			if elementsCount != test.expectedNumUpdates {
				t.Errorf("did not get the expected number of elements wanted: %v got: %v", test.expectedNumUpdates, elementsCount)
			}
		})
	}
}
