package main

import (
	"errors"
	"github.com/akrylysov/pogreb"
	"log"
	"time"
)

type CacheStorage struct {
	db *pogreb.DB
}

func NewCacheStorage() *CacheStorage {
	db, err := pogreb.Open("cache", nil)
	if err != nil {
		log.Fatalln(err)
	}
	return &CacheStorage{
		db: db,
	}
}

func (cs *CacheStorage) Get(key string) ([]byte, error) {
	return cs.db.Get([]byte(key))
}

func (cs *CacheStorage) Set(key string, value []byte, exp time.Duration) error {
	return cs.db.Put([]byte(key), value)
}

func (cs *CacheStorage) Delete(key string) error {
	return cs.db.Delete([]byte(key))
}

func (cs *CacheStorage) Reset() error {
	it := cs.db.Items()
	for {
		k, _, e := it.Next()
		if errors.Is(e, pogreb.ErrIterationDone) {
			break
		}
		if e != nil {
			return e
		}
		cs.db.Delete(k)
	}
	return nil
}

func (cs *CacheStorage) Close() error {
	return cs.db.Close()
}
