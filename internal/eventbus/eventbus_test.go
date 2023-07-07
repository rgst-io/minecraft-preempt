// Copyright (C) 2023 Jared Allard <jared@rgst.io>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package eventbus implements a global eventbus that can be used
// to register events and react to them.
package eventbus_test

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jaredallard/minecraft-preempt/internal/eventbus"
	"gotest.tools/v3/assert"
)

type eventTest struct {
	data         any
	returnValues []any
	wantErr      error
}

type customType struct {
	echo string
}

var customEventKey eventbus.EventKey = "customEventKey"

func TestInstance_RegisterHandlerEmit(t *testing.T) {
	tests := []struct {
		name     string
		handlers map[eventbus.EventKey][]eventbus.EventHandler
		events   map[eventbus.EventKey]eventTest
	}{
		{
			name: "should process a single event",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld"): {
					func(data any) (any, error) {
						return "Hello World!", nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld"): {
					data:         "",
					returnValues: []any{"Hello World!"},
				},
			},
		},
		{
			name: "should process and event with multiple handlers",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld"): {
					func(data any) (any, error) {
						return "Hello World!", nil
					},
					func(data any) (any, error) {
						return "Hello World 2!", nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld"): {
					data:         "",
					returnValues: []any{"Hello World!", "Hello World 2!"},
				},
			},
		},
		{
			name: "should process an event with complicated input and ouput",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("complicatedType"): {
					func(c customType) (customType, error) {
						return c, nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("complicatedType"): {
					data:         customType{echo: "hi"},
					returnValues: []any{customType{echo: "hi"}},
				},
			},
		},
		{
			name: "should allow custom eventkey",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				customEventKey: {
					func(i string) (string, error) {
						return "echo: " + i, nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				customEventKey: {
					data:         "hi",
					returnValues: []any{"echo: hi"},
				},
			},
		},
		{
			name: "should handle distinct handlers",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld1"): {
					func(i string) (string, error) {
						return "echo: " + i, nil
					},
				},
				eventbus.EventKey("helloWorld2"): {
					func(i string) (string, error) {
						return "necho: " + i, nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld1"): {
					data:         "hi",
					returnValues: []any{"echo: hi"},
				},
				eventbus.EventKey("helloWorld2"): {
					data:         "hi",
					returnValues: []any{"necho: hi"},
				},
			},
		},
		{
			name: "should handle error",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld"): {
					func(i string) (string, error) {
						return "", fmt.Errorf("it all broke")
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld"): {
					data:    "",
					wantErr: fmt.Errorf("it all broke"),
				},
			},
		},
		{
			name: "should support only error return value",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld"): {
					func(i string) error {
						return fmt.Errorf("it all broke")
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld"): {
					data:    "",
					wantErr: fmt.Errorf("it all broke"),
				},
			},
		},
		{
			name: "should support only error return value (nil error)",
			handlers: map[eventbus.EventKey][]eventbus.EventHandler{
				eventbus.EventKey("helloWorld"): {
					func(i string) error {
						return nil
					},
				},
			},
			events: map[eventbus.EventKey]eventTest{
				eventbus.EventKey("helloWorld"): {
					data: "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eb := eventbus.New()

			// register the provided handlers
			for k, hs := range tt.handlers {
				for _, h := range hs {
					t.Logf("Registering handler for %q", k)
					assert.NilError(t, eb.RegisterHandler(k, h))
				}
			}

			// emit the events
			for k, e := range tt.events {
				t.Logf("Emitting event %q with data: %v", k, e.data)
				rtrn, err := eb.Emit(k, e.data)

				if e.wantErr == nil {
					assert.NilError(t, err)
					assert.DeepEqual(t, rtrn, e.returnValues, cmp.AllowUnexported(customType{}))
				} else {
					assert.ErrorContains(t, err, e.wantErr.Error())
				}
			}
		})
	}
}
