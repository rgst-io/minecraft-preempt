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
package eventbus

import "reflect"

// global is the global eventbus. Use New() to create
// an eventbus that is not global.
var global = New()

// EventKey is a key for handling or emitting an event. This
// must be unique across the entire application for a distinct
// datatype.
type EventKey string

// EventHandler is a function the handles an event. It should follow
// one of the "generic" signatures of:
//
//	func(any) (any, error)
//	func(any) error
type EventHandler any

// Instance is an eventbus. See package comment, as well
// as receiver comments for an idea of what this is meant
// to do.
type Instance struct {
	handlers map[EventKey][]EventHandler
}

// New creates a new eventbus instance that does not
// use the global instance.
func New() *Instance {
	return &Instance{
		handlers: make(map[EventKey][]EventHandler),
	}
}

// RegisterHandler registers an event handler for the provided
// EventKey.
//
// Currently, error is always nil. It is set in case an error ever
// needs to be returned here. As such, be sure to always handle
// it accordingly.
func (i *Instance) RegisterHandler(k EventKey, e EventHandler) error {
	i.handlers[k] = append(i.handlers[k], e)
	return nil
}

// Emit emits an event at the specified event key and waits
// for all handlers to respond. If they return any data, that data
// will be returned as the []any return value of this function. If
// any of the handlers return an error, no further handlers will be
// executed and the error will be returned.
func (i *Instance) Emit(k EventKey, data any) ([]any, error) {
	var results []any
	for _, handler := range i.handlers[k] {
		hr := reflect.ValueOf(handler)

		// TODO(jaredallard): Move into RegisterHandler.
		// Ensure we're a function and that we're not nil
		if hr.Kind() != reflect.Func || hr.IsNil() {
			continue
		}

		rtrn := hr.Call([]reflect.Value{reflect.ValueOf(data)})

		var result any
		switch len(rtrn) {
		case 0: // No data, don't append anything
		case 1: // Should just be an error value
			rtrn0Inf := rtrn[0].Interface()
			err, ok := rtrn0Inf.(error)
			if ok && err != nil { // had an error
				return nil, err
			}
		case 2: // Arbitrary data with error value
			rtrn1Inf := rtrn[1].Interface()
			err, ok := rtrn1Inf.(error)
			if ok && err != nil { // had an error
				return nil, err
			}

			result = rtrn[0].Interface()
		}
		if result == nil {
			continue
		}

		results = append(results, result)
	}

	return results, nil
}

// Emit calls Emit() n the global eventbus instance.
func Emit(k EventKey, data any) ([]any, error) {
	return global.Emit(k, data)
}

// RegisterHandler calls RegisterHandler() on the global
// eventbus instance.
func RegisterHandler(k EventKey, e EventHandler) error {
	return global.RegisterHandler(k, e)
}
