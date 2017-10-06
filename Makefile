# Copyright 2017 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

CLOSURE_JAR := $(HOME)/Downloads/closure-compiler-v20170910.jar
JS_SOURCES := js/dom_updater.js js/stream_client.js js/streaminghdp.js js/log.js $(wildcard js/json/*)
JS_EXTERNS := js/client_stub_extern.js
STATIC_DIR := static
STATIC_FILE := $(STATIC_DIR)/streaming_hdp.js

all: go_build $(STATIC_FILE)

go_build:
	go build streaming_hdp/...

$(STATIC_FILE): $(JS_SOURCES) $(JS_EXTERNS)
	java -jar $(CLOSURE_JAR) --compilation_level ADVANCED_OPTIMIZATIONS --js $(JS_SOURCES) --externs $(JS_EXTERNS) --js_output_file $(STATIC_DIR)/streaming_hdp.js
