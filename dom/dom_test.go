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

package dom

import (
	"fmt"
	"reflect"
	"testing"

	"streaming_hdp/dom/domjson"
)

// Helper for generating a list of attributes in the form of key followed by value
// associated to the key. This reflects the input from DevTools.
func generateInputAttributes(size int, suffix string) []interface{} {
	attributes := make([]interface{}, 0)
	for i := 0; i < size; i++ {
		attributes = append(attributes, fmt.Sprintf("foo%s%d", suffix, i))
		attributes = append(attributes, fmt.Sprintf("bar%s%d", suffix, i))
	}
	return attributes
}

// Helper for generating a map of attributes and its associated value.
func generateExpectedAttributes(size int, suffix string) map[string]string {
	attributes := map[string]string{}
	for i := 0; i < size; i++ {
		attributes[fmt.Sprintf("foo%s%d", suffix, i)] = fmt.Sprintf("bar%s%d", suffix, i)
	}
	return attributes
}

func TestGenerateInitialDOM(t *testing.T) {
	// Tests initial version of the DOM tree.
	// The expected result is in the form of slice of updates events.
	// Things that are checking includes parent --> child mapping,
	// previous sibling mapping, and the number of updates.
	tests := []struct {
		dom                map[string]interface{}
		expectedNumUpdates int
		label              string
		expected           []*domjson.DOMUpdate
	}{
		{
			label: "General",
			dom: map[string]interface{}{
				NodeID:        float64(1), // Needs to be float64 to mirror DevTools output.
				BackendNodeID: float64(1),
				NodeValue:     "a",
				LocalName:     "a",
				NodeName:      "a",
				Attributes:    generateInputAttributes(1, "0"),
				Children: []interface{}{
					map[string]interface{}{
						NodeID:        float64(2),
						BackendNodeID: float64(2),
						NodeValue:     "b",
						LocalName:     "b",
						NodeName:      "b",
						Attributes:    generateInputAttributes(1, "1"),
						Children:      []interface{}{},
					},
					map[string]interface{}{
						NodeID:        float64(3),
						BackendNodeID: float64(3),
						NodeValue:     "c",
						LocalName:     "c",
						NodeName:      "c",
						Attributes:    generateInputAttributes(1, "2"),
						Children:      []interface{}{},
					},
				},
			},
			expected: []*domjson.DOMUpdate{
				&domjson.DOMUpdate{
					Action: domjson.Insert,
					Node: domjson.Node{
						NodeID:         "1",
						ParentNodeID:   "",
						PreviousNodeID: "",
						ElementType:    "a",
						Attributes:     generateExpectedAttributes(1, "0"),
						Text:           "a",
					},
				},
				&domjson.DOMUpdate{
					Action: domjson.Insert,
					Node: domjson.Node{
						NodeID:         "2",
						ParentNodeID:   "1",
						PreviousNodeID: "",
						ElementType:    "b",
						Attributes:     generateExpectedAttributes(1, "1"),
						Text:           "b",
					},
				},
				&domjson.DOMUpdate{
					Action: domjson.Insert,
					Node: domjson.Node{
						NodeID:         "3",
						ParentNodeID:   "1",
						PreviousNodeID: "2",
						ElementType:    "c",
						Attributes:     generateExpectedAttributes(1, "2"),
						Text:           "c",
					},
				},
			},
		},
	}
	for _, test := range tests {
		domModel := NewDOMModel()
		t.Run(test.label, func(t *testing.T) {
			result, err := domModel.GenerateInitialDOM(Node(test.dom))
			if err != nil {
				t.Errorf("error generating the initial DOM: %v", err)
			}
			if len(result) != len(test.expected) {
				t.Errorf("expected: %v updates, but got %v with updates: %v", test.expectedNumUpdates, len(result), result)
			}
			for i, r := range result {
				if ok := reflect.DeepEqual(test.expected[i], r); !ok {
					t.Errorf("incorrect message wanted: %#v got: %#v", test.expected[i], r)
				}
			}
		})
	}
}

// Test the conversion of the object from DevTools format to the protobuf format.
func TestGenerateModificationUpdate(t *testing.T) {
	// Defines the type of the update.
	type UpdateType string

	const (
		updateNode      UpdateType = "update_node"
		removeNode      UpdateType = "remove_node"
		updateAttribute UpdateType = "update_attribute"
	)

	tests := []struct {
		element    map[string]interface{}
		label      string
		updateType UpdateType
		expected   *domjson.DOMUpdate
	}{
		{
			label:      "Insert first node",
			updateType: updateNode,
			element: map[string]interface{}{
				ParentNodeID:   float64(0),
				PreviousNodeID: float64(0),
				NodeField: map[string]interface{}{
					NodeID:        float64(4),
					BackendNodeID: float64(4),
					NodeValue:     "first",
					NodeName:      "first",
				},
			},
			expected: &domjson.DOMUpdate{
				Action: domjson.Insert,
				Node: domjson.Node{
					NodeID:         "4",
					ParentNodeID:   "",
					PreviousNodeID: "",
					ElementType:    "first",
					Attributes:     map[string]string{},
					Text:           "first",
				},
			},
		},
		{
			label:      "Insert second node",
			updateType: updateNode,
			element: map[string]interface{}{
				ParentNodeID:   float64(4),
				PreviousNodeID: float64(0),
				NodeField: map[string]interface{}{
					NodeID:        float64(1),
					BackendNodeID: float64(1),
					NodeValue:     "a",
					NodeName:      "a",
				},
			},
			expected: &domjson.DOMUpdate{
				Action: domjson.Insert,
				Node: domjson.Node{
					NodeID:         "1",
					ParentNodeID:   "4",
					PreviousNodeID: "",
					ElementType:    "a",
					Attributes:     map[string]string{},
					Text:           "a",
				},
			},
		},
		{
			label:      "Attribute Update",
			updateType: updateAttribute,
			element: map[string]interface{}{
				NodeID: float64(1),
				Name:   "foo",
				Value:  "bar",
			},
			expected: &domjson.DOMUpdate{
				Action: domjson.Modify,
				Node: domjson.Node{
					NodeID: "1",
					Attributes: map[string]string{
						"foo": "bar",
					},
				},
			},
		},
		{
			label:      "Remove one node",
			updateType: removeNode,
			element: map[string]interface{}{
				NodeID:       float64(1),
				ParentNodeID: float64(4),
			},
			expected: &domjson.DOMUpdate{
				Action: domjson.Remove,
				Node: domjson.Node{
					NodeID:       "1",
					ParentNodeID: "4",
				},
			},
		},
	}
	domModel := NewDOMModel()
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			var result *domjson.DOMUpdate
			var err error
			switch test.updateType {
			case updateNode:
				result, err = domModel.ProcessNodeInsertion(Node(test.element))
				if err != nil {
					t.Errorf("error processing node insertion: %v", err)
				}
			case removeNode:
				result, err = domModel.ProcessNodeRemoval(Node(test.element))
				if err != nil {
					t.Errorf("error processing node removal: %v", err)
				}
			case updateAttribute:
				result, err = domModel.ProcessNodeAttributeModification(Node(test.element))
				if err != nil {
					t.Errorf("error processing node removal: %v", err)
				}
			default:
				t.Fatalf("testing a not supported modification")
			}

			if ok := reflect.DeepEqual(test.expected, result); !ok {
				t.Errorf("incorrect message wanted: %#v got: %#v", test.expected, result)
			}
		})
	}
}
