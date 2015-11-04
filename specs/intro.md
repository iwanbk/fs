# intro

- is read only caching fuse based virtual filesystem
- every file read can optionally be cached in ayfs_stor's
- an ayfs_stor can be installed locally or remotely

# definitions


### aysfs_client (CL)

* fuse client
* exposes the filesystem to local OS
* uses list of aysfs_stor's to find files 
* metadata comes from ays


# components

## aysfs_cl

### config

```python

[main]
id="aUniqueIdForThisClient"


[[ays.cache]]
#cache at gridlevel, can have more than 1, will try all till found what it needs
mnt="/mnt/gridnode1/"

[[ays.cache]]
mnt="http://192.168.1.1:8080"

[[ays.stor]]
#cache at gridlevel, can have more than 1, will try all till found what it needs
url="http://stor1.aydo.com:8080"
cache.expiration = 0

[[ays.stor]]
url="ssh://stor2.aydo.com:8080/mnt/1"
cache.expiration = 0

[[debug]]
debug_filter = [".*\.py",".*\.hrd"]
redis.addr="127.0.0.1"
redis.port=555
redis.passwd=""

[[ays]]
id="jumpscale|mongodb!main"
prefetch.cache.grid=0
prefetch.cache.local=0
cache.grid=1
cache.local=1

[[ays]]
id="jumpscale|mysql!main"
prefetch.cache.grid=0
prefetch.cache.local=0

```

### features

- autoreload the config file
- is readonly (exception for debug feature)
- support easy mechanism for developer to work on files in reality and get feedback to his local machine

### how are files & metadata stored in boltdb

- metadata
    - hset `md:$ays_id` key:`dirpath` val:`["filename":"$hash,$size",...]`
    - no need to store things like acl's: root:root will always have access moddate is now

- files
    - hset `files:$dedupedomain` key:`hash` val:`binarydata`  


### debug

- we allow write behaviour in debug mode
- for files specified in debug_filter (use specified redis)
    - file paths will be remembered in 
        - hset `debug:paths:$mountname` key:`$path` 
        - value = md5_hashkey,size
    - files will be cached in in 
        - hset `debug:files:$mountname` key:`$hash` 
        - value = md5_hashkey,size
    - transaction log in
        - llist `debug:log:$mountname` append [epoch,"$path","$hash"]
        - will use remotely to fetch changed files and download them locally on local e.g. github for further processing of changed data
- if file requested at fuselayer need to check debug first
    - if debug on & regex matches
        - check in configured redis if path is there, if so read hash
        - get file from files location
        - no need to go to store

### remarks


# remarks
- all in golang
- use fuse (golang) to expose local fs
- caching using bolt
