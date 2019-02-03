// +build ignore

package main

import (
	"github.com/shurcooL/vfsgen"
	"log"
	"net/http"
)

func main() {

	var fs = http.Dir("data")

	err := vfsgen.Generate(fs, vfsgen.Options{BuildTags: "!dev"})

	if err != nil {
		log.Fatal(err)
	}
}
