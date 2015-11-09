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
	ID     string
	Boltdb string
}

type AYS struct {
	ID                 string
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
	URL         string
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
