# goxtag
[![Build Status](https://travis-ci.org/azlotnikov/goxtag.svg?branch=master)](https://travis-ci.org/azlotnikov/goxtag)
[![Coverage Status](https://coveralls.io/repos/github/azlotnikov/goxtag/badge.svg?branch=master)](https://coveralls.io/github/azlotnikov/goxtag?branch=master)

## Example

```go
package main

import (
    "github.com/azlotnikov/goxtag"
    "log"
    "net/http"
)

// Structured representation for github file name table
type example struct {
    Title string `xpath:"//h1"`
    Files []string `xpath:".//table[contains(concat(' ',normalize-space(@class),' '),' files ')]//tbody//tr[contains(concat(' ',normalize-space(@class),' '),' js-navigation-item ')]//td[contains(concat(' ',normalize-space(@class),' '),' content ')]"`
}

func main() {
    res, err := http.Get("https://github.com/azlotnikov/goxtag")
    if err != nil {
        log.Fatal(err)
    }
    defer res.Body.Close()

    var ex example
	
    err = goxtag.NewDecoder(res.Body).Decode(&ex)
    if err != nil {
        log.Fatal(err)
    }

    log.Println(ex.Title, ex.Files)
}
```

## Details

### goxtag
This package is an analog of [github.com/andrewstuart/goq](https://github.com/andrewstuart/goq) for xpath selectors.
