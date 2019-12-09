package goxtag

import (
	"bytes"
	"golang.org/x/net/html"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type Unmarshaler interface {
	UnmarshalHTML([]*html.Node) error
}

type valFunc func(doc *Document) string

type xpathTag struct {
	tag      string
	required bool
}

const (
	tagName     = "xpath"
	ignoreTag   = "!ignore"
	requiredTag = "xpath_required"
)

var (
	textVal valFunc = func(doc *Document) string {
		return strings.TrimSpace(doc.Text())
	}

	vfMut   = sync.Mutex{}
	vfCache = map[string]valFunc{}
)

func (tag xpathTag) valFunc() valFunc {
	vfMut.Lock()
	defer vfMut.Unlock()

	if fn := vfCache[tag.tag]; fn != nil {
		return fn
	}

	f := textVal

	vfCache[tag.tag] = f
	return f
}

// Unmarshal takes a byte slice and a destination pointer to any
// interface{}, and unmarshals the document into the destination based on the
// rules above. Any error returned here will likely be of type
// CannotUnmarshalError, though an initial htmlquery error will pass through
// directly.
func Unmarshal(bs []byte, v interface{}) error {
	root, err := html.Parse(bytes.NewReader(bs))

	if err != nil {
		return err
	}

	return UnmarshalSelection(newDocumentWithNode(root), v)
}

func wrapUnmErr(err error, v reflect.Value) error {
	if err == nil {
		return nil
	}

	return &CannotUnmarshalError{
		V:      v,
		Reason: customUnmarshalError,
		Err:    err,
	}
}

func UnmarshalSelection(doc *Document, iface interface{}) error {
	v := reflect.ValueOf(iface)

	// Must come before v.IsNil() else IsNil panics on NonPointer value
	if v.Kind() != reflect.Ptr {
		return &CannotUnmarshalError{V: v, Reason: nonPointer}
	}

	if iface == nil || v.IsNil() {
		return &CannotUnmarshalError{V: v, Reason: nilValue}
	}

	u, v := indirect(v)

	if u != nil {
		return wrapUnmErr(u.UnmarshalHTML(doc.Nodes), v)
	}

	return unmarshalByType(doc, v, xpathTag{})
}

func unmarshalByType(doc *Document, v reflect.Value, tag xpathTag) error {
	u, v := indirect(v)

	if u != nil {
		return wrapUnmErr(u.UnmarshalHTML(doc.Nodes), v)
	}

	// Handle special cases where we can just set the value directly
	switch val := v.Interface().(type) {
	case []*html.Node:
		val = append(val, doc.Nodes...)
		v.Set(reflect.ValueOf(val))
		return nil
	}

	t := v.Type()

	switch t.Kind() {
	case reflect.Struct:
		return unmarshalStruct(doc, v)
	case reflect.Slice:
		return unmarshalSlice(doc, v, tag)
	case reflect.Array:
		return unmarshalArray(doc, v, tag)
	case reflect.Map:
		return &CannotUnmarshalError{
			V:      v,
			Reason: mapIsNotSupportedError,
		}
	default:
		vf := tag.valFunc()
		str := vf(doc)
		err := unmarshalLiteral(str, v, tag.required)
		if err != nil {
			return &CannotUnmarshalError{
				V:      v,
				Reason: typeConversionError,
				Err:    err,
				Val:    str,
			}
		}
		return nil
	}
}

func unmarshalLiteral(s string, v reflect.Value, required bool) error {
	t := v.Type()

	switch t.Kind() {
	case reflect.Interface:
		if t.NumMethod() == 0 {
			// For empty interfaces, just set to a string
			nv := reflect.New(reflect.TypeOf(s)).Elem()
			nv.Set(reflect.ValueOf(s))
			v.Set(nv)
		}
	case reflect.Bool:
		i, err := strconv.ParseBool(s)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetBool(i)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		i, err := strconv.ParseFloat(s, 64)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetFloat(i)
	case reflect.String:
		v.SetString(s)
	}
	return nil
}

func unmarshalStruct(doc *Document, v reflect.Value) error {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		tag := xpathTag{
			tag:      t.Field(i).Tag.Get(tagName),
			required: true,
		}

		if tag.tag == ignoreTag {
			continue
		}

		if tag.tag == "" {
			if u, _ := indirect(v.Field(i)); u == nil {
				continue
			}
		}

		// If tag is empty and the object doesn't implement Unmarshaler, skip
		if tag.tag == "" {
			if u, _ := indirect(v.Field(i)); u == nil {
				continue
			}
		}

		required := t.Field(i).Tag.Get(requiredTag)
		if required != "" {
			var err error
			tag.required, err = strconv.ParseBool(required)
			if err != nil {
				return err
			}
		}

		sel := doc
		if tag.tag != "" {
			selStr := tag.tag
			sel = doc.Find(selStr)
		}

		if err := unmarshalByType(sel, v.Field(i), tag); err != nil {
			return &CannotUnmarshalError{
				Reason:   typeConversionError,
				Err:      err,
				V:        v,
				FldOrIdx: t.Field(i).Name,
			}
		}
	}
	return nil
}

func unmarshalArray(doc *Document, v reflect.Value, tag xpathTag) error {
	if v.Type().Len() != len(doc.Nodes) {
		return &CannotUnmarshalError{
			Reason: arrayLengthMismatch,
			V:      v,
		}
	}

	for i := 0; i < v.Type().Len(); i++ {
		err := unmarshalByType(doc.Eq(i), v.Index(i), tag)
		if err != nil {
			return &CannotUnmarshalError{
				Reason:   typeConversionError,
				Err:      err,
				V:        v,
				FldOrIdx: i,
			}
		}
	}

	return nil
}

func unmarshalSlice(doc *Document, v reflect.Value, tag xpathTag) error {
	slice := v
	eleT := v.Type().Elem()

	for i := 0; i < doc.Length(); i++ {
		newV := reflect.New(TypeDeref(eleT))

		err := unmarshalByType(doc.Eq(i), newV, tag)

		if err != nil {
			return &CannotUnmarshalError{
				Reason:   typeConversionError,
				Err:      err,
				V:        v,
				FldOrIdx: i,
			}
		}

		if eleT.Kind() != reflect.Ptr {
			newV = newV.Elem()
		}

		v = reflect.Append(v, newV)
	}

	slice.Set(v)
	return nil
}
