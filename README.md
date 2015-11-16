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

# grid level cache
[[cache]]
    mnt="/mnt/store1"

# global stores
[[store]]
    url="http://ays_store"
```

## starting fuse layer
mounting the fuse layer at /opt  
```./aysfs -config config.toml /opt```

to enable pprof tool, add the -pprof flag to the command  
```./aysfs -pprof /opt```  
and go to http://localhost:6060/debug/pprof