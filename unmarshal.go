package goxtag

import (
	"bytes"
	"golang.org/x/net/html"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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
	ignoreTag   = "-"
	requiredTag = "xpath_required"
)

var (
	textVal valFunc = func(doc *Document) string {
		return strings.TrimSpace(doc.Text())
	}
	indexRegEx = regexp.MustCompile(`\[\d+\]$`)
)

func (tag *xpathTag) valFunc() valFunc {
	return textVal
}

func (tag *xpathTag) hasIndex() bool {
	return indexRegEx.MatchString(tag.tag)
}

func (tag *xpathTag) hasSuffix(s string) bool {
	return strings.HasSuffix(tag.tag, s)
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

	return UnmarshalSelection(NewDocumentWithNode(root), v)
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
		return &CannotUnmarshalError{
			V:      v,
			Reason: nonPointer,
		}
	}

	if iface == nil || v.IsNil() {
		return &CannotUnmarshalError{
			V:      v,
			Reason: nilDestination,
		}
	}

	u, v := indirect(v)

	if u != nil {
		return wrapUnmErr(u.UnmarshalHTML(doc.Nodes), v)
	}

	return unmarshalByType(doc, v, xpathTag{})
}

func findByTag(doc *Document, tag xpathTag) (*Document, error) {
	if tag.tag != "" {
		return doc.Find(tag.tag), nil
	}
	return doc, nil
}

func findOneByTag(doc *Document, tag xpathTag) (*Document, error) {
	if tag.tag != "" {
		return doc.FindOne(tag.tag)
	}
	return doc, nil
}

func findForTypeByTag(doc *Document, v reflect.Value, tag xpathTag) (*Document, error) {
	var sel *Document
	var err error
	hasIndex := tag.hasIndex()
	hasTextSuffix := tag.hasSuffix("text()")
	switch {
	case hasIndex && !hasTextSuffix:
		sel, err = findOneByTag(doc, tag)
	default:
		sel, err = findByTag(doc, tag)
	}
	if err != nil {
		return nil, err
	}

	t := v.Type()
	//type may have custom Unmarshal, check unsupported types later
	switch t.Kind() {
	case reflect.Struct:
		return sel, nil
	case reflect.Slice:
		return sel, nil
	case reflect.Array:
		return sel, nil
	case reflect.Map:
		return sel, nil
	case reflect.Interface:
		return sel, nil
	case reflect.Ptr:
		return sel, nil
	default:
		if hasIndex || hasTextSuffix {
			return sel, nil
		}
		_sel, err := findByTag(doc, tag)
		if err != nil {
			return nil, err
		}
		if _sel.Length() > 1 {
			return nil, &CannotUnmarshalError{
				V:      v,
				Reason: multipleNodesDetected,
				XPath:  tag.tag,
			}
		}
		return sel, nil
	}
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
			XPath:  tag.tag,
		}
	default:
		vf := tag.valFunc()
		str := vf(doc)
		err := unmarshalLiteral(str, v, tag.required)
		if err != nil {
			return &CannotUnmarshalError{
				V:      v,
				Reason: typeConversionError,
				XPath:  tag.tag,
				Err:    err,
				Val:    str,
			}
		}
		return nil
	}
}

func unmarshalLiteral(s string, v reflect.Value, required bool) error {
	t := v.Type()

	trimmedValue := strings.TrimSpace(s)
	switch t.Kind() {
	case reflect.Interface:
		if t.NumMethod() == 0 {
			// For empty interfaces, just set to a string
			nv := reflect.New(reflect.TypeOf(s)).Elem()
			nv.Set(reflect.ValueOf(s))
			v.Set(nv)
		}
	case reflect.Bool:
		i, err := strconv.ParseBool(trimmedValue)
		if err != nil {
			return err
		}
		v.SetBool(i)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if trimmedValue == "" {
			return nil
		}
		i, err := strconv.ParseInt(trimmedValue, 10, 64)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if trimmedValue == "" {
			return nil
		}
		i, err := strconv.ParseUint(trimmedValue, 10, 64)
		if err != nil {
			if required {
				return err
			}
			return nil
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		if trimmedValue == "" {
			return nil
		}
		i, err := strconv.ParseFloat(trimmedValue, 64)
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

		sel, err := findForTypeByTag(doc, v.Field(i), tag)
		if err != nil {
			return err
		}

		if !tag.required && sel.IsEmpty() {
			continue
		}

		if sel.IsEmpty() {
			return &CannotUnmarshalError{
				V:      v,
				Reason: nodeNotFound,
				XPath:  tag.tag,
			}
		}

		if err := unmarshalByType(sel, v.Field(i), tag); err != nil {
			return &CannotUnmarshalError{
				V:        v,
				Reason:   typeConversionError,
				XPath:    tag.tag,
				Err:      err,
				FldOrIdx: t.Field(i).Name,
			}
		}
	}
	return nil
}

func unmarshalArray(doc *Document, v reflect.Value, tag xpathTag) error {
	if v.Type().Len() != len(doc.Nodes) {
		return &CannotUnmarshalError{
			V:      v,
			Reason: arrayLengthMismatch,
			XPath:  tag.tag,
		}
	}

	for i := 0; i < v.Type().Len(); i++ {
		err := unmarshalByType(doc.Eq(i), v.Index(i), tag)
		if err != nil {
			return &CannotUnmarshalError{
				V:        v,
				Reason:   typeConversionError,
				XPath:    tag.tag,
				Err:      err,
				FldOrIdx: i,
			}
		}
	}

	return nil
}

func unmarshalSlice(doc *Document, v reflect.Value, tag xpathTag) error {
	slice := v
	eleT := v.Type().Elem()

	v.SetLen(0)
	for i := 0; i < doc.Length(); i++ {
		newV := reflect.New(TypeDeref(eleT))

		err := unmarshalByType(doc.Eq(i), newV, tag)

		if err != nil {
			return &CannotUnmarshalError{
				V:        v,
				Reason:   typeConversionError,
				XPath:    tag.tag,
				Err:      err,
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
