// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "encoding/json"

// cloneNode deep-copies a node by way of JSON, which is what a node is.
func cloneNode(n Node) Node {
	var out Node
	b, err := json.Marshal(n)
	if err != nil || json.Unmarshal(b, &out) != nil {
		return n
	}
	return out
}

func cloneDocument(d Document) Document {
	var out Document
	b, err := json.Marshal(d)
	if err != nil || json.Unmarshal(b, &out) != nil {
		return d
	}
	return out
}

// ClonePage deep-copies a page.
func ClonePage(p Page) Page {
	var out Page
	b, err := json.Marshal(p)
	if err != nil || json.Unmarshal(b, &out) != nil {
		return p
	}
	return out
}
