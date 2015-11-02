# intro

- is read only caching fuse based virtual filesystem
- every file read can optionally be cached in ayfs_stor's
- an ayfs_stor can be installed locally or remotely

# definitions


### aysfs_client (CL)

* fuse client
* exposes the filesystem to local OS
* uses list of aysfs_stor's to find files & metadata

### ayfs_stor  (ST)

* ledis based stor for files & metata



# components

## aysfs_cl

### config

```python

[main]
id="aUniqueIdForThisClient"

[[cache.local]]
#is local ledis instance, working over tcp locally
#if port not specified or this section not specified do not use
port=999
#time in hours before files get expired if not fetched at least once (0 means do not expire)
expiration = 48

[[cache.grid]]
#cache at gridlevel, can have more than 1, will try all till found what it needs
#http based with optional login/passwd
url="http://192.168.2.1:8080"
login=""
passwd=""

[[cache.grid]]
#cache at gridlevel
#http based with optional login/passwd
url="http://192.168.2.2:8080"
login=""
passwd=""

[[ays.stor]]
name="jumpscale"
url="https://files.jumpscale.org:8080/files"
login=""
secret=""
#time in hours before files get expired if not fetched at least once (0 means do not expire)
cache.expiration = 0

[[ays.stor]]
name="ovc"
url="https://files.ovc.aydo.com:8080/files"
login=""
secret=""
cache.expiration = 48

[[ays.stor]]
name="customerx"
url="https://files.ovc.aydo.com:8080/custx"
login="custx"
secret="1234"

[[mount_files]]
name=mongodb
path="/opt/jumpscale7/apps/mongodb"
platform="ubuntu64"
#always with version & no instance (is for template files)
ays.id="jumpscale|mongodb(10.1.888)"
#ays.repoid is normally empty because is template, if not empty then this means ays recipe has been defined on this level
ays.repoid=""
stor.name="jumpscale"
#next is for easy debugging purposes, can write back to developer 
debug = "kdsdebug"
debug_filter = [".*\.py",".*\.hrd"]
#default is empty
dedupe_domain = ""
prefetch=1 

[[mount_config]]
path="/opt/jumpscale7/apps/mongodb/cfg"
remote="cfg"
#always with instance !!!
ays.id="jumpscale|mongodb!main"
#is unique id to full repo path of this ays repo
ays.repoid="1SdY294"
stor.name="customerx"
dedupe_domain = "" 

[[debug]]
name="kdsdebug"
#is normal redis (not ledis)
#use std redis client 
redis.addr="127.0.0.1"
redis.port=555
redis.passwd=""
```

### features

- autoreload the config file
- is readonly (exception for debug feature)
- support easy mechanism for developer to work on files in reality and get feedback to his local machine

### how are files & metadata stored in ledis

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



## ays_cache

- http://ledisdb.com/ based
- use bolt as backend db
- each aysf client connects to this stor over http(s)
- is a HTTP(S) server which uses local/remote ledis store as backend

### how
- when aysfs_client starts it tells about its existence to all known ays_caches (clientid & platform will be remembered on cache)
- when aysfs_client starts (or reloads config) it will let aysfs_stor know which store's are known
    - post $stor_url/ays/stor/$name payload is url,login,passwd,expiration 
        - info from config file in aysfs_client
        - the longest expiration is used when the storname is already there (so client asking for longest expiration wins)
    - this is stored in local ledis
- when aysfs_client starts (or reloads config) it will let aysfs_stor know which mounts are known
    - post $stor_url/ays/mount/$clientid/$mountname payload is info in the toml file for that mount as dict
    - this is stored in local ledis    
- when aysfs_client needs metadata
    - get  $stor_url/md/$clientid/$mountname/$path?fetch=1
        - path is subpath of that ays
        - if fetch=0 then do not try to fetch from stores remotely configured
        - if fetch = 1 fetch and wait
        - if fetch = 2 fetch async return ""
        - if fetch = 3 fetch and wait but do not cache
    - return "" if not found, so cache does not have the info
- when aysfs_client needs file
    - get  $stor_url/$dedupedomain/$hash?fetch=1
    - hashed files are stored as 
        - store as messagepack???
    - each dedupe domain is in separate db file (ledis) ???


### config

```python
[[ledis]]
port=999

```

# remarks
- all in golang
- use fuse (golang) to expose local fs
- caching using bolt
