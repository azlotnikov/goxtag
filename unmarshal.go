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

type xpathTag string

const (
	tagName   = "xpath"
	ignoreTag = "!ignore"
)

var (
	textVal valFunc = func(doc *Document) string {
		return strings.TrimSpace(doc.Text())
	}
	htmlVal = func(doc *Document) string {
		str, _ := doc.Html()
		return strings.TrimSpace(str)
	}

	vfMut   = sync.Mutex{}
	vfCache = map[xpathTag]valFunc{}
)

func attrFunc(attr string) valFunc {
	return func(doc *Document) string {
		str, _ := doc.Attr(attr)
		return str
	}
}

func (tag xpathTag) valFunc() valFunc {
	vfMut.Lock()
	defer vfMut.Unlock()

	if fn := vfCache[tag]; fn != nil {
		return fn
	}

	srcArr := strings.Split(string(tag), ",")
	if len(srcArr) < 2 {
		vfCache[tag] = textVal
		return textVal
	}

	src := srcArr[1]

	var f valFunc
	switch {
	case src[0] == '[':
		// [someattr] will return value of .Attr("someattr")
		attr := src[1 : len(src)-1]
		f = attrFunc(attr)
	case src == "html":
		f = htmlVal
	case src == "text":
		f = textVal
	default:
		f = textVal
	}

	vfCache[tag] = f
	return f
}

// popVal should allow us to handle arbitrarily nested maps as well as the
// cleanly handling the possiblity of map[literal]literal by just delegating
// back to `unmarshalByType`.
func (tag xpathTag) popVal() xpathTag {
	arr := strings.Split(string(tag), ",")
	if len(arr) < 2 {
		return tag
	}
	newA := []string{arr[0]}
	newA = append(newA, arr[2:]...)

	return xpathTag(strings.Join(newA, ","))
}

// Unmarshal takes a byte slice and a destination pointer to any
// interface{}, and unmarshals the document into the destination based on the
// rules above. Any error returned here will likely be of type
// CannotUnmarshalError, though an initial goquery error will pass through
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

// UnmarshalSelection will unmarshal a goquery.goquery.Selection into an interface
// appropriately annoated with goquery tags.
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

	return unmarshalByType(doc, v, "")
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
		err := unmarshalLiteral(str, v)
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

func unmarshalLiteral(s string, v reflect.Value) error {
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
			return err
		}
		v.SetBool(i)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		i, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
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
		tag := xpathTag(t.Field(i).Tag.Get(tagName))

		if tag == ignoreTag {
			continue
		}

		if tag == "" {
			if u, _ := indirect(v.Field(i)); u == nil {
				continue
			}
		}

		// If tag is empty and the object doesn't implement Unmarshaler, skip
		if tag == "" {
			if u, _ := indirect(v.Field(i)); u == nil {
				continue
			}
		}

		sel := doc
		if tag != "" {
			selStr := string(tag)
			sel = doc.Find(selStr)
		}

		err := unmarshalByType(sel, v.Field(i), tag)
		if err != nil {
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
