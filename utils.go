package main

import (
	"log"
	"net/url"
	"strings"
)

func ParseRequest(cookies string, splitter string) url.Values {
	resp := url.Values{}
	list := strings.Split(cookies, splitter)
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		log.Println(entry)
		vals := strings.SplitN(entry, "=", 2)
		val := ""
		if len(vals) == 2 {
			val = vals[1]
		}
		resp.Add(vals[0], val)
	}
	return resp
}
