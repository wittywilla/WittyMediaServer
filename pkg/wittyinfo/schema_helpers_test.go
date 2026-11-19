// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"os"
	"reflect"
)

func readSchema() ([]byte, error) { return os.ReadFile("../../docs/formats/wittyinfo.schema.json") }

func typeOfInfo() reflect.Type { return reflect.TypeOf(Info{}) }
