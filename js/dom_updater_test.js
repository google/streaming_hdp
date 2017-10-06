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
 * @fileoverview This is the test file for the DOM updater class.
 */

goog.module('streaminghdp.js.DOMUpdaterTest');
goog.setTestOnly();

const Action = goog.require('streaminghdp.js.json.Action');
const DOMUpdate = goog.require('streaminghdp.js.json.DOMUpdate');
const DOMUpdater = goog.require('streaminghdp.js.DOMUpdater');
const DOMUpdates = goog.require('streaminghdp.js.json.DOMUpdates');
const Node = goog.require('streaminghdp.Node');
const dom = goog.require('goog.dom');
const testSuite = goog.require('goog.testing.testSuite');
goog.require('goog.testing.asserts');

let domUpdater;

testSuite({
  setUp: function() {
    domUpdater = new DOMUpdater();
    // Make sure that we know about the BODY.
    const body = createNodeUpdate(Action.INSERT, '42');
    body.getNode().setElementType('BODY');
    const messageEvent = createDOMUpdates(body);
    domUpdater.handleUpdates(messageEvent);
  },
  tearDown: function() {
    // Clear the body.
    dom.removeChildren(dom.getElementsByTagName('BODY')[0]);
  },
  testInsert: function() {
    const update = createNodeUpdate(Action.INSERT, '44');
    update.getNode().setElementType('div');
    update.getNode().setParentNodeId('42');
    update.getNode().setPreviousNodeId('');
    update.getNode().setText('');
    update.getNode().getAttributesMap().set('id', 'inserted_node');

    const domUpdates = createDOMUpdates(update);

    domUpdater.handleUpdates(domUpdates);
    assertEquals(
        1, dom.getChildren(dom.getElementsByTagName('BODY')[0]).length);
    assertNotNull(dom.getElement('inserted_node'));
  },
  testInsertToRootDocument: function() {
    const documentNode = createNodeUpdate(Action.INSERT, '40');
    documentNode.getNode().setElementType('#document');

    const htmlNode = createNodeUpdate(Action.INSERT, '41');
    htmlNode.getNode().setElementType('html');

    const update = createNodeUpdate(Action.INSERT, '44');
    update.getNode().setElementType('#comment');
    update.getNode().setParentNodeId('40');
    update.getNode().setPreviousNodeId('41');
    update.getNode().setText('hello world!');

    const domUpdates = createDOMUpdates(documentNode, htmlNode, update);

    domUpdater.handleUpdates(domUpdates);
    const html = dom.getElementsByTagName('html')[0];
    assertTrue(html.nextSibling.nodeType === 8);  // 8 is the COMMENT_NODE
  },
  testRemoval: function() {
    const insert = createNodeUpdate(Action.INSERT, '44');
    insert.getNode().setElementType('div');
    insert.getNode().setParentNodeId('42');
    insert.getNode().setPreviousNodeId('');
    insert.getNode().setText('');
    insert.getNode().getAttributesMap().set('id', 'inserted_node');

    const removal = createNodeUpdate(Action.REMOVE, '44');
    removal.getNode().setParentNodeId('42');

    const domUpdates = createDOMUpdates(insert, removal);

    domUpdater.handleUpdates(domUpdates);
    assertEquals(
        0, dom.getChildren(dom.getElementsByTagName('BODY')[0]).length);
  },
});

/**
 * Helper function for creating an DOM node update. This only
 * populates action, nodeId, and elementType fields.
 *
 * @param {Action} action The action for the DOM node update.
 * @param {string} nodeID The ID of the node.
 * @param {string} elementType The type of the DOM node.
 * @return {DOMUpdate}
 */
function createNodeUpdate(action, nodeID, elementType) {
  const node = new Node();
  node.nodeID(nodeID);
  const nodeUpdate = new DOMUpdate(action, node);
  nodeUpdate.setAction(action);
  nodeUpdate.setNode(node);
  return nodeUpdate;
}

/**
 * Helper function for creating a event message containing the DOM updates.
 *
 * @param {...DOMUpdate} updates The dom updates.
 * action.
 * @return {DOMUpdates}
 */
function createDOMUpdates(...updates) {
  const resultUpdates = new DOMUpdates();
  resultUpdates.setDomUpdatesList(updates);
  return resultUpdates;
}
