// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxDocumentBytes is the largest encoded document Unmarshal accepts.
const MaxDocumentBytes = 2 << 20 // 2 MiB

// ErrTooLarge is returned by Unmarshal when the input exceeds MaxDocumentBytes.
var ErrTooLarge = errors.New("sdui: document exceeds 2 MiB")

// ErrVersion is returned (wrapped) by Unmarshal when the document's version
// is not SchemaVersion.
var ErrVersion = errors.New("sdui: unsupported schema version")

// Marshal encodes doc as compact JSON. It does not validate.
func Marshal(doc Document) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// MarshalIndent encodes doc as indented JSON for humans and golden files.
func MarshalIndent(doc Document) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Unmarshal decodes a document strictly: unknown fields are rejected, input
// larger than MaxDocumentBytes is rejected with ErrTooLarge, trailing data is
// rejected, and the version must equal SchemaVersion (ErrVersion). It does
// not run Validate; callers that accept documents from users should.
func Unmarshal(data []byte) (Document, error) {
	if len(data) > MaxDocumentBytes {
		return Document{}, ErrTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("sdui: decode: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Document{}, errors.New("sdui: decode: trailing data after document")
	}
	if doc.Version != SchemaVersion {
		return Document{}, fmt.Errorf("%w: got %d, want %d", ErrVersion, doc.Version, SchemaVersion)
	}
	return doc, nil
}
