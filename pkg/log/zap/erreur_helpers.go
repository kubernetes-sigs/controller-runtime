/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package zap

import (
	"bytes"
	"fmt"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/erreur"
	"golang.org/x/exp/errors"
)

// ErreurAwareEncoder is an structured-error-aware Zap Encoder.  It adds
// structure as extra fields, and sets the stack if available.  Instead of
// trying to force Erreur errors objects to implement ObjectMarshaller, we just
// implement a wrapper Encoder that checks for pkg/erreur types.
type ErreurAwareEncoder struct {
	// Encoder is the zapcore.Encoder that this encoder delegates to
	zapcore.Encoder

	// Verbose indicates if stack traces should be printed.
	Verbose bool
}

// NB(directxman12): can't just override AddReflected, since the encoder calls AddReflected on itself directly

// Clone implements zapcore.Encoder
func (k *ErreurAwareEncoder) Clone() zapcore.Encoder {
	return &ErreurAwareEncoder{
		Encoder: k.Encoder.Clone(),
	}
}

// EncodeEntry implements zapcore.Encoder
func (k *ErreurAwareEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	for i, field := range fields {
		if field.Type != zapcore.ErrorType {
			continue
		}

		structured, isStructured := field.Interface.(erreur.Structured)
		if !isStructured {
			continue
		}
		// NB: commented-out code is a slightly less efficient way of doing things,
		// but closer to what zap normally looks like
		/*valRecv := &fieldValRecv{fields: fields}
		structured.ValuesInto(valRecv)
		fields = valRecv.fields*/
		fields[i] = zap.Object(field.Key, structuredMarshaller{err: field.Interface.(error), structured: structured, unwrappable: field.Interface.(errors.Wrapper)})

		/*if unwrappable, canUnwrap := field.Interface.(errors.Wrapper); canUnwrap && unwrappable.Unwrap() != nil {
			fields = append(fields, zap.Array("errorCauses", causeArrayMarshaller{wrapped: unwrappable}))
		}*/

		// TODO(directxman12): do we really want to override the stack like this?
		withTrace, hasTrace := field.Interface.(erreur.HasCaller)
		if !hasTrace {
			continue
		}
		trace := withTrace.Caller()
		if trace == (errors.Frame{}) {
			continue
		}
		traceOut := bufferPrinter{
			detail: k.Verbose,
		}
		trace.Format(&traceOut)
		entry.Stack = traceOut.Buffer.String()
	}

	return k.Encoder.EncodeEntry(entry, fields)
}

// fieldValRecv is an erreur.ValueReceiver that appends zapcore.Fields.
type fieldValRecv struct {
	fields []zapcore.Field
}

func (r *fieldValRecv) Pair(key string, val interface{}) {
	r.fields = append(r.fields, zap.Any(key, val))
}

type bufferPrinter struct{
	bytes.Buffer
	detail bool
}
func (p *bufferPrinter) Print(args ...interface{}) {
	fmt.Fprint(&p.Buffer, args...)
}
func (p *bufferPrinter) Printf(format string, args ...interface{}) {
	fmt.Fprintf(&p.Buffer, format, args...)
}
func (p *bufferPrinter) Detail() bool {
	return p.detail
}

// causeArrayMarshaller fakes multi-error support using the same key Zap does,
// but without requiring allocations, etc.
type causeArrayMarshaller struct {
	wrapped errors.Wrapper
}
func (c causeArrayMarshaller) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	next := c.wrapped.Unwrap()
	for next != nil {
		enc.AppendString(next.Error())
		unwrappable, canUnwrap := next.(errors.Wrapper)
		if !canUnwrap {
			next = nil
			continue
		}
		next = unwrappable.Unwrap()
	}

	return nil
}

type structuredMarshaller struct {
	err error
	structured erreur.Structured
	unwrappable errors.Wrapper
}

func (s structuredMarshaller) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("message", s.err.Error())
	s.structured.ValuesInto(encValRecv{enc: enc})
	if s.unwrappable != nil && s.unwrappable.Unwrap() != nil {
		if err := enc.AddArray("causes", causeArrayMarshaller{wrapped: s.unwrappable}); err != nil {
			return nil
		}
	}
	return nil
}

type encValRecv struct {
	enc zapcore.ObjectEncoder
}
func (e encValRecv) Pair(key string, val interface{}) {
	e.enc.AddReflected(key, val)
}
