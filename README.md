# aysfs
caching filesystem which will be used by ays, mainly used to deploy applications in a grid

# How to
config file example
```
[main]
    id = "dev"

# defines what services you want to use
[[ays]]
    id="jumpscale__base"
[[ays]]
    id="jumpscale__mongodb"

# Cache layers
[[cache]]
    url="/mnt/store1"
    purge=true

[[cache]]
    url="/mnt/store2"

[[cache]]
    url="http://ays_store"
```

## Caches layers:
Caches are quereid in the order of the definition above. a cach must define a URL to the files location. In case of folders
on local machine, the url can be defined as absolute path or as `file:///path` syntax.
*Purge* option works on caches that has write access and if true, this cache will be wiped clean before aysfs starts. 

Cache layers that supports writing will get populated with files that are found in higher layers of cache.

## starting fuse layer
mounting the fuse layer at /opt  
```./aysfs -config config.toml /opt```

to enable pprof tool, add the -pprof flag to the command  
```./aysfs -pprof /opt```  
and go to http://localhost:6060/debug/pprof
