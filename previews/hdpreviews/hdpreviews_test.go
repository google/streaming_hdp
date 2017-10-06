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

package hdpreviews

import (
	"strings"
	"testing"
)

// Checks if the string contains an event handler string.
func containsEventHandler(s string) bool {
	var jsEventHandlerSet = map[string]struct{}{
		"onload":      {},
		"onerror":     {},
		"onchange":    {},
		"onclick":     {},
		"onmouseover": {},
		"onmouseout":  {},
	}
	for eventHandler := range jsEventHandlerSet {
		if strings.Contains(eventHandler, s) {
			return true
		}
	}
	return false
}

// Tests that <script> tags are removed from the HTML.
func TestRemoveScriptTags(t *testing.T) {
	tests := []struct {
		label string
		dom   string
	}{
		{
			label: "General",
			dom: `<html>
			<head><script src="foo_2.js"></script></head>
			<body><script src="foo.js"></script><div>bar</div></body>
			</html>`,
		},
		{
			label: "No <script> tag exists from the beginning",
			dom: `<html>
			<body><div>bar</div></body>
			</html>`,
		},
		{
			label: "<script> tags in iframes",
			dom: `<html>
			<head></head>
			<body>
			<div>bar</div>
			<iframe><html><body><script src="iframe.js"></script></body></html></iframe>
			</body>
			</html>`,
		},
		{
			label: "HTML contains event handlers",
			dom: `<html>
			<head></head>
			<body onerror="error()">
			<div>bar</div>
			<iframe><html><body onload="foo()"></body></html></iframe>
			</body>
			</html>`,
		},
		{
			label: "HTML contains event handlers and <script> tags",
			dom: `<html>
			<head></head>
			<body onerror="error()">
			<div>bar<script src="foo.js"></script></div>
			<iframe><html><body onload="foo()"></body></html></iframe>
			</body>
			</html>`,
		},
		{
			label: "HTML contains event handlers, other attributes and <script> tags",
			dom: `<html>
			<head></head>
			<body onerror="error()" style="background-color: red">
			<div>bar<script src="foo.js"></script></div>
			<iframe><html><body onload="foo()"></body></html></iframe>
			</body>
			</html>`,
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			dom := test.dom
			result, err := removeScriptTags(dom)
			if err != nil {
				t.Errorf("Test: %v Failed; Error removing script tags: %v",
					test.label, err)
			}
			// TODO(vaspol): make the test more robust: parse tree and look for script
			// elements and event handlers.
			if strings.Contains("</script>", result) || containsEventHandler(result) {
				t.Errorf("Test: %v Failed; Script tag exists in respBody: %v",
					test.label, result)
			}
		})
	}
}

// TODO(vaspol): add test for isDocument()
