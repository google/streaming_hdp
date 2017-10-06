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

// Proxy intercepts HTML requests, calls the rendering service (Chrome Browser)
// for those URLs, strips the script tags from the DOM and returns this in
// the response. This essentially executes javascript on the server rather
// than the client (but breaks interactivity and personalization).
// Usage:
// blaze run :streaminghdpreviewsproxy -- --cert_file=$PWD/data/cert.pem --key_file=$PWD/data/key.pem
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"streaming_hdp/chrome"
	"streaming_hdp/previews/streaminghdpreviews"
	"streaming_hdp/previews/streaminghdpreviews/stream"
)

var (
	proxyHost     = flag.String("proxy_host", "localhost", "The host that the proxy is running on.")
	port          = flag.Int("port", 8080, "The port the proxy will listen to.")
	certFile      = flag.String("cert_file", "mycert.pem", "The SSL certificate file.")
	keyFile       = flag.String("key_file", "mykey.pem", "The SSL key file.")
	verbose       = flag.Bool("verbose", false, "Enable verbose output.")
	useFullChrome = flag.Bool("use_full_chrome", false, "Runs Chrome with the graphical interface.")
	staticDir     = flag.String("static_dir", "static", "The directory where the static HTML and JavaScript files can be found.")
)

func main() {
	flag.Parse()

	chromeInstanceManager := chrome.NewInstanceManager(*useFullChrome)
	hdpHandler, err := streaminghdpreviews.New(*proxyHost, *port, chromeInstanceManager, *staticDir)
	if err != nil {
		log.Fatalf("Failed to create HD Previews handler: %v\n", err)
	}
	http.Handle("/", hdpHandler)

	streamHandler, err := stream.New(chromeInstanceManager, *verbose)
	if err != nil {
		log.Fatalf("failed to create stream handler  %v", err)
	}
	http.Handle("/stream", streamHandler)

	server := &http.Server{
		Addr: fmt.Sprintf(":%d", *port),
	}
	log.Fatal(server.ListenAndServeTLS(*certFile, *keyFile))
}
