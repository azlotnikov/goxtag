package goxtag

import (
	"golang.org/x/net/html"
	"io"
)

// Decoder implements the same API you will see in encoding/xml and
// encoding/json except that we do not currently support proper streaming
// decoding as it is not supported by goquery upstream.
type Decoder struct {
	err     error
	topNode *html.Node
}

// NewDecoder returns a new decoder given an io.Reader
func NewDecoder(r io.Reader) *Decoder {
	d := &Decoder{}
	d.topNode, d.err = html.Parse(r)
	return d
}

// Decode will unmarshal the contents of the decoder when given an instance of
// an annotated type as its argument. It will return any errors encountered
// during either parsing the document or unmarshaling into the given object.
func (d *Decoder) Decode(dest interface{}) error {
	if d.err != nil {
		return d.err
	}
	if d.topNode == nil {
		return &CannotUnmarshalError{
			Reason: "resulting document was nil",
		}
	}

	return UnmarshalSelection(newDocumentWithNode(d.topNode), dest)
}
