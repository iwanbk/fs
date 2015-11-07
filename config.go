package main

import "time"

type Config struct {
	Main Main

	Ays []AYS

	Cache []Cache
	Store []Store

	Debug []Debug
}

type Main struct {
	Id     string
	boltdb string
}

type AYS struct {
	Id                 string
	PrefetchCacheGrid  bool
	PrefetchCacheLocal bool
	CacheLocal         bool
	CacheGrid          bool
}

type Cache struct {
	Mnt         string
	Expirtation time.Duration
}

type Store struct {
	Url         string
	Expirtation time.Duration
}

type Debug struct {
	DebugFilter []string
	Redis       Redis
}

type Redis struct {
	Addr     string
	Port     int
	Password string
}
