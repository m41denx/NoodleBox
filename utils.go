package main

import (
	"log"
	"net/url"
	"os"
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

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func GetEnv(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func GetKVEnv(key string) map[string]string {
	val := os.Getenv(key)
	if val == "" {
		return map[string]string{}
	}
	kv := map[string]string{}
	for _, v := range strings.Split(val, ",") {
		kv[strings.SplitN(v, "=", 2)[0]] = strings.SplitN(v, "=", 2)[1]
	}
	return kv
}
