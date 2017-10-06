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

/**
 * @fileoverview Implements the logic for monitoring updates to the DOM for
 * Streaming HD Previews via Stream API. The client connects to a SHDP proxy
 * via Stream API and the proxy sends back updates in JSPB format to this
 * client. This client will pass the messages to the DOMUpdater accordingly.
 */

// TODO(vaspol): Add proper test for StreamClient.
goog.module('streaminghdp.js.StreamClient');

const DOMUpdates = goog.require('streaminghdp.js.json.DOMUpdates');

class StreamClient {
  constructor(url, id, domUpdater) {
    this.url_ = 'http://' + url + '/stream?id=' + id;
    this.domUpdater_ = domUpdater;

    // Start a connection to the stream endpoint on the proxy. This will be the
    // channel to receive the updates which will be sent from the server over
    // the stream.
    fetch(this.url_)
        .then((response) => {
          const reader = /** @type {!ReadableStreamDefaultReader} */
              (response.body.getReader());
          let partial = '';
          const decoder = new TextDecoder();

          // Search is called recursively to handle the data streaming from the
          // proxy until the proxy finishes loading the page.
          const search = () => {
            return reader.read().then((result) => {
              partial += decoder.decode(
                  /** @type {!ArrayBuffer} */ (result.value) ||
                      new Uint8Array(0),
                  {stream: !result.done});

              var delim = /(?:\r|\r\n)/;
              var completeUpdates = partial.split(delim);

              if (!result.done) {
                // Keep hold of the partial update until the next call.
                partial = completeUpdates[completeUpdates.length - 1];
                completeUpdates = completeUpdates.slice(0, -1);
              }

              for (var update of completeUpdates) {
                update = update.trim();  // This is a json string.
                const domUpdates =
                    /** @type {DOMUpdates} */ (JSON.parse(update));
                this.domUpdater_.handleUpdates(domUpdates);
              }

              if (result.done) {
                return null;
              }

              return search();  // we are not done yet. recurse to process the
                                // next chunk of data from the stream.
            });
          };

          return search();
        })
        .catch(function(err) {
          console.log(err.message);
        });
  }
}

exports = StreamClient;
