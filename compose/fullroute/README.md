# Benchmark for GoBGP and RustyBGP to receiving full routes

The log about received routes is written to `/var/log/watcher.log` on the fullroute_server_1 container.

## GoBGP

```
$ docker-compose -f docker-compose-gobgp.yml up --scale client=8
```


## RustyBGP 

```
$ docker-compose -f docker-compose-rusty.yml up --scale client=8
```
