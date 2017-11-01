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
//

/**
 * @fileoverview This file defines the DOMUpdate object.
 *
 * @suppress {reportUnknownTypes}
 */

goog.module('streaminghdp.js.json.DOMUpdate');

const Action = goog.require('streaminghdp.js.json.Action');
const Node = goog.require('streaminghdp.js.json.Node');

class DOMUpdate {
  constructor(action, node) {
    /** @const {!Action} */
    this.Action = action;

    /** @const {!Node} */
    this.Node = node;
  }
}

exports = DOMUpdate;
