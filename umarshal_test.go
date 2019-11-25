package goxtag

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
	"strconv"
	"testing"
)

const testPage = `<!DOCTYPE html>
<html>
  <head>
    <title></title>
    <meta charset="utf-8" />
  </head>
  <body>
    <h1>
      <ul id="resources">
        <li class="resource" order="3">
          <div class="name">Foo</div>
        </li>
        <li class="resource" order="1">
          <div class="name">Bar</div>
        </li>
        <li class="resource" order="4">
          <div class="name">Baz</div>
        </li>
        <li class="resource" order="2">
          <div class="name">Bang</div>
        </li>
        <li class="resource" order="5">
          <div class="name">Zip</div>
        </li>
      </ul>
	  <div class="name"></div>
      <h2 id="anchor-header"><a href="https://foo.com">FOO!!!</a></h2>
    </h1>
		<ul id="structured-list">
		  <li name="foo" val="flip">foo</li>
			<li name="bar" val="flip">bar</li>
			<li name="baz" val="flip">baz</li>
		</ul>
		<ul id="nested-map">
			<ul name="first">
				<li name="foo">foo</li>
				<li name="bar">bar</li>
				<li name="baz">baz</li>
			</ul>
			<ul name="second">
				<li name="bang">bang</li>
				<li name="ring">ring</li>
				<li name="fling">fling</li>
			</ul>
		</ul>
		<div class="foobar">
			<thing foo="yes">1</thing>
			<foo arr="true">true</foo>
			<bar arr="true">false</bar>
			<float>1.2345</float>
			<int>-123</int>
			<uint>100</uint>
		</div>
  </body>
</html>
`

var vals = []string{"Foo", "Bar", "Baz", "Bang", "Zip"}

type ErrorFooBar struct{}

var errTestUnmarshal = fmt.Errorf("A wild error appeared")

func (e *ErrorFooBar) UnmarshalHTML([]*html.Node) error {
	return errTestUnmarshal
}

type Page struct {
	Resources []Resource `xpath:".//*[@id='resources']//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]"`
	FooBar    FooBar
}

type Resource struct {
	Name string `xpath:".//*[contains(concat(' ',normalize-space(@class),' '),' name ')]"`
}

type FooBar struct {
	Attrs              []Attr
	Val                int
	unmarshalWasCalled bool
}

type Attr struct {
	Key, Value string
}

func (f *FooBar) UnmarshalHTML(nodes []*html.Node) error {
	f.unmarshalWasCalled = true

	s := newDocumentWithNodes(nodes)

	f.Attrs = []Attr{}
	for _, node := range s.Find(".//*[contains(concat(' ',normalize-space(@class),' '),' foobar ')]//thing").Nodes {
		for _, attr := range node.Attr {
			f.Attrs = append(f.Attrs, Attr{Key: attr.Key, Value: attr.Val})
		}
	}
	thing := s.Find(".//thing")

	thingText := thing.Text()

	i, err := strconv.Atoi(thingText)
	f.Val = i
	return err
}

func TestArrayUnmarshal(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Resources [5]Resource `xpath:".//*[@id='resources']//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]"`
	}

	asrt.NoError(Unmarshal([]byte(testPage), &a))
	for i, val := range vals {
		asrt.Equal(val, a.Resources[i].Name)
	}
}

func TestBoolean(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		BoolTest struct {
			Foo bool `xpath:".//foo"`
			Bar bool `xpath:".//bar"`
		} `xpath:".//*[contains(concat(' ',normalize-space(@class),' '),' foobar ')]"`
	}

	asrt.NoError(Unmarshal([]byte(testPage), &a))

	asrt.Equal(true, a.BoolTest.Foo)
	asrt.Equal(false, a.BoolTest.Bar)
}

func TestNumbers(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		BoolTest struct {
			Int   int     `xpath:".//int"`
			Float float32 `xpath:".//float"`
			Uint  uint16  `xpath:".//uint"`
		} `xpath:".//*[contains(concat(' ',normalize-space(@class),' '),' foobar ')]"`
	}

	asrt.NoError(Unmarshal([]byte(testPage), &a))

	asrt.Equal(float32(1.2345), a.BoolTest.Float)
	asrt.Equal(-123, a.BoolTest.Int)
	asrt.Equal(uint16(100), a.BoolTest.Uint)
}

func TestAttributes(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Orders []int `xpath:".//*[@id='resources']//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]/@order"`
	}

	asrt.NoError(Unmarshal([]byte(testPage), &a))
	assert.Equal(t, []int{3, 1, 4, 2, 5}, a.Orders)
}

func TestUnmarshal(t *testing.T) {
	asrt := assert.New(t)

	asrt.Implements((*Unmarshaler)(nil), new(FooBar))

	var p Page

	asrt.NoError(Unmarshal([]byte(testPage), &p))
	asrt.Len(p.Resources, 5)

	for i, val := range vals {
		asrt.Equal(val, p.Resources[i].Name)
	}

	asrt.True(p.FooBar.unmarshalWasCalled, "Unmarshal should have been called.")
	asrt.Equal(1, p.FooBar.Val)
	asrt.Len(p.FooBar.Attrs, 1)
	asrt.Equal("foo", p.FooBar.Attrs[0].Key)
	asrt.Equal("yes", p.FooBar.Attrs[0].Value)
}

func TestUnmarshalError(t *testing.T) {
	asrt := assert.New(t)

	var a []ErrorFooBar

	err := Unmarshal([]byte(testPage), &a)

	asrt.Contains(err.Error(), "[]goxtag.ErrorFooBar[0]")

	e := checkErr(asrt, err)
	e2 := checkErr(asrt, e.Err)

	asrt.Equal(errTestUnmarshal, e2.Err)
	asrt.Equal(customUnmarshalError, e2.Reason)
}

func TestNilUnmarshal(t *testing.T) {
	asrt := assert.New(t)

	var a *Page

	err := Unmarshal([]byte{}, a)
	e := checkErr(asrt, err)
	asrt.Equal(nilValue, e.Reason)
}

func TestNonPointer(t *testing.T) {
	asrt := assert.New(t)

	var a Page
	e := checkErr(asrt, Unmarshal([]byte{}, a))
	asrt.Equal(nonPointer, e.Reason)
}

func TestWrongArrayLength(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Resources [1]Resource `xpath:".//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]"`
	}

	err := Unmarshal([]byte(testPage), &a)

	e := checkErr(asrt, err)
	asrt.Equal(typeConversionError, e.Reason)
	e2 := checkErr(asrt, e.Err)
	asrt.Equal(arrayLengthMismatch, e2.Reason)

	asrt.Contains(e.Error(), "Resource")
	asrt.Contains(e.Error(), "array length")
}

func TestInvalidLiteral(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Foo int `xpath:".//foo"`
	}

	err := Unmarshal([]byte(testPage), &a)

	e := checkErr(asrt, err).unwind()

	asrt.Len(e.chain, 2)
	asrt.Error(e.tail)
	asrt.Contains(err.Error(), e.tail.Error())
	asrt.Contains(err.Error(), "\"true\"")
	asrt.Equal("true", e.val)

	asrt.Equal(typeConversionError, e.chain[0].Reason)
	asrt.Equal(typeConversionError, e.chain[1].Reason)
}

func TestInvalidArrayEleType(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Resources [5]int `xpath:".//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]"`
	}

	err := Unmarshal([]byte(testPage), &a)
	e := checkErr(asrt, err).unwind()
	asrt.Len(e.chain, 3)
}

func TestDirectInsertion(t *testing.T) {
	asrt := assert.New(t)

	var a struct {
		Nodes []*html.Node `xpath:".//ul[@id='resources']//*[contains(concat(' ',normalize-space(@class),' '),' resource ')]"`
	}

	asrt.NoError(Unmarshal([]byte(testPage), &a))
	asrt.Len(a.Nodes, 5)
}

func TestInterfaceDecode(t *testing.T) {
	asrt := assert.New(t)
	var a struct {
		IF interface{} `xpath:".//*[@id='structured-list']//li"`
	}
	asrt.NoError(Unmarshal([]byte(testPage), &a))
	asrt.Equal("foobarbaz", a.IF.(string))
}

func checkErr(asrt *assert.Assertions, err error) *CannotUnmarshalError {
	asrt.Error(err)
	asrt.IsType((*CannotUnmarshalError)(nil), err)

	return err.(*CannotUnmarshalError)
}
