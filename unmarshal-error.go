package goxtag

import (
	"fmt"
	"reflect"
)

// All "Reason" fields within CannotUnmarshalError will be constants and part of
// this list
const (
	nonPointer             = "non-pointer value"
	nodeNotFound           = "node not found in document"
	nilDestination         = "destination is nil"
	arrayLengthMismatch    = "array length does not match document elements found"
	customUnmarshalError   = "a custom Unmarshaler implementation threw an error"
	typeConversionError    = "a type conversion error occurred"
	mapIsNotSupportedError = "map type is not currently supported"
	multipleNodesDetected  = "multiple nodes detected for selector"
)

// CannotUnmarshalError represents an error returned by the goqxtag Unmarshaler
// and helps consumers in programmatically diagnosing the cause of their error.
type CannotUnmarshalError struct {
	Err      error
	Val      string
	FldOrIdx interface{}
	V        reflect.Value
	Reason   string
	XPath    string
}

// This type is a mid-level abstraction to help understand the error printing logic
type errChain struct {
	chain []*CannotUnmarshalError
	val   string
	tail  error
}

// tPath returns the type path in the same string format one might use to access
// the nested value in go code. This should hopefully help make debugging easier.
func (e errChain) tPath() string {
	nest := ""

	for _, err := range e.chain {
		if err.FldOrIdx != nil {
			switch nesting := err.FldOrIdx.(type) {
			case string:
				switch err.V.Type().Kind() {
				case reflect.Map:
					nest += fmt.Sprintf("[%q]", nesting)
				case reflect.Struct:
					nest += fmt.Sprintf(".%s", nesting)
				}
			case int:
				nest += fmt.Sprintf("[%d]", nesting)
			case *int:
				nest += fmt.Sprintf("[%d]", *nesting)
			default:
				nest += fmt.Sprintf("[%v]", nesting)
			}
		}
	}

	return nest
}

func (e errChain) last() *CannotUnmarshalError {
	return e.chain[len(e.chain)-1]
}

// Error gives a human-readable error message for debugging purposes.
func (e errChain) Error() string {
	last := e.last()

	// Avoid panic if we cannot get a type name for the Value
	t := "unknown: invalid value"
	if last.V.IsValid() {
		t = last.V.Type().String()
	}

	msg := "could not unmarshal "

	if e.val != "" {
		msg += fmt.Sprintf("value %q ", e.val)
	}

	v := e.chain[0].V

	if v.CanAddr() {
		msg += fmt.Sprintf(
			"into '%s%s' (type %s): %s",
			v.Type(),
			e.tPath(),
			t,
			last.Reason,
		)
	}

	if last.XPath != "" {
		msg += fmt.Sprintf(" tag: '%s'", last.XPath)
	}

	// If a generic error was reported elsewhere, report its message last
	if e.tail != nil {
		msg = msg + ": " + e.tail.Error()
	}

	return msg
}

// Traverse e.Err, printing hopefully helpful type info until there are no more
// chained errors.
func (e *CannotUnmarshalError) unwind() *errChain {
	str := &errChain{chain: []*CannotUnmarshalError{}}
	for {
		str.chain = append(str.chain, e)

		if e.Val != "" {
			str.val = e.Val
		}

		// Terminal error was of type *CannotUnmarshalError and had no children
		if e.Err == nil {
			return str
		}

		if e2, ok := e.Err.(*CannotUnmarshalError); ok {
			e = e2
			continue
		}

		// Child error was not a *CannotUnmarshalError; print its message
		str.tail = e.Err
		return str
	}
}

func (e *CannotUnmarshalError) Error() string {
	return e.unwind().Error()
}
