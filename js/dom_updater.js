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
 * @fileoverview Implements the library for monitoring updates to the DOM via
 * WebSocket.
 */

goog.module('streaminghdp.js.DOMUpdater');

/** @define {boolean} */
goog.define('STREAMINGHDP_DOMUPDATER_DEBUG_DOM', true);

const Action = goog.require('streaminghdp.js.json.Action');
const DOMUpdates = goog.require('streaminghdp.js.json.DOMUpdates');
const JSONNode = goog.require('streaminghdp.js.json.Node');
const log = goog.require('streaminghdp.js.log');

class DOMUpdater {
  constructor() {
    /**
     * The map for DOM node lookup.
     * @private {!Map}
     */
    this.domNodes_ = new Map();
  }

  /**
   * Handles DOM updates in the form of DOMUpdates jspb objects.
   *
   * @param {DOMUpdates} updates The updates to be applied to the DOM.
   */
  handleUpdates(updates) {
    const updatesList = updates.Updates;
    updatesList.forEach((update) => {
      log.verbose('update: ' + update);

      // TODO(vaspol): MOVE action is just remove then insert for now.
      try {
        switch (update.Action) {
          case Action.INSERT: {
            this.processInsert_(update.Node);
            break;
          }
          case Action.REMOVE: {
            this.processRemove_(update.Node);
            break;
          }
          case Action.MODIFY: {
            this.processAttributeChange_(update.Node);
            break;
          }
        }
      } catch (err) {
        console.log(err);
      }
    });
  }

  /**
   * Generates a DOM node from the given node information.
   *
   * @param {JSONNode} node the node to
   * be generated.
   * @return {Element|Text|Comment} a new node based on the information given in the
   * node argument.
   * @private
   */
  generateDomNode_(node) {
    let newNode;
    if (node.ElementType == '#text') {
      newNode = document.createTextNode(node.Text);
    } else if (node.ElementType == '#comment') {
      newNode = document.createComment(node.Text);
    } else {
      newNode = document.createElement(node.ElementType);
      const attributes = node.Attributes;
      for (const attribute in attributes) {
        try {
          // Some pages have invalid attributes. Some times
          // these attributes are not critical to rendering the page.
          // We can try to absorb the errors and log them, if
          // the attributes are actually critical to the visuals
          // of the page.
          newNode.setAttribute(attribute, attributes[attribute]);
        } catch (err) {
          console.log(err);
        }
      };
    }
    return newNode;
  }

  /**
   * Modifies the attributes of the given node in the DOM tree.
   *
   * @param {JSONNode} node The node to
   * be modified.
   * @private
   */
  processAttributeChange_(node) {
    log.verbose('processing attribute change for ' + node);
    const targetNode = this.domNodes_.get(node.NodeID);
    log.verbose('attribute change target node: ' + targetNode);

    // TODO(vaspol): find a cleaner way to access the map key.
    const attributes = node.Attributes;
    const name = Object.keys(attributes)[0];
    const value = attributes[name];
    targetNode.setAttribute(name, value);
  }

  /**
   * Inserts the given node into the DOM.
   *
   * @param {JSONNode} node The node to
   * be inserted into the DOM.
   * @private
   */
  processInsert_(node) {
    // 3 cases to handle:
    //   (1) left-most child: just call on first-child.
    //   (2) not the left-most child: have to insert after the prev node.
    //   (3) special default HTML nodes: HTML, HEAD, BODY. Don't insert a node,
    //   but just add the node to the mapping.
    log.verbose('processing insert for ' + node);

    // TODO(vaspol): This if statement is a hack for an issue where some DOM
    // nodes are being inserted twice. When a node is inserted twice, one of the
    // nodes will not contain any of the children resulting in an exception when
    // inserting a child to the duplicated node.
    if (!this.domNodes_.has(node.NodeID)) {
      let newDomNode;
      switch (node.ElementType.toLowerCase()) {
        case '#document': {
          // There are 2 cases to handle for document element.
          //  (1) rootElement of the page: just get the documentElement of the
          // document.
          //  (2) iframeRootElement of the page: get the parent and find the
          // document element from there.
          if (node.ParentNodeID == '') {
            newDomNode = document;
          } else {
            const parentNode = this.domNodes_.get(node.ParentNodeID);
            newDomNode =
                parentNode.contentDocument || parentNode.contentWindow.document;
          }
          if (typeof newDomNode === 'undefined') {
            throw new Error('could not find the #document node');
          }
          break;
        }
        case 'html':
        case 'body':
        case 'head': {
          newDomNode = document.getElementsByTagName(node.ElementType)[0];
          if (typeof newDomNode !== 'undefined') {
            break;
          }
          // Fall through
        }
        default: {
          newDomNode = this.generateDomNode_(node);
          const parentNode = this.domNodes_.get(node.ParentNodeID);
          let referenceNode;
          if (node.PreviousNodeID == '') {
            referenceNode = parentNode.firstChild;
          } else {
            // Gets the previous node's next sibling to insert before the next
            // sibling node.
            referenceNode = this.domNodes_.get(node.PreviousNodeID).nextSibling;
          }
          parentNode.insertBefore(newDomNode, referenceNode);
          break;
        }
      }
      this.domNodes_.set(node.NodeID, newDomNode);
      if (STREAMINGHDP_DOMUPDATER_DEBUG_DOM == 1) {
        if (typeof newDomNode.setAttribute === 'function') {
          newDomNode.setAttribute('x-shdp-dom-id', node.NodeID);
        }
      }
      log.verbose(this.domNodes_.toString());
    } else {
      log.verbose('Got duplicated node with id: ' + node.NodeID);
    }
  }

  /**
   * Removes the given node from the DOM.
   *
   * @param {JSONNode} node The node to be
   *     removed from the DOM.
   * @private
   */
  processRemove_(node) {
    log.verbose('processing remove for ' + node);
    const targetNodeID = node.NodeID;
    const targetNode = this.domNodes_.get(targetNodeID);
    if (typeof targetNode !== 'undefined') {
      targetNode.remove();
      this.domNodes_.delete(targetNodeID);
    }
  }
}

exports = DOMUpdater;
