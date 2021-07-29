# bgp update watcher

This is monitoring tool on BGP daemon of sending and receiving update messages. Supported BGP software are [GoBGP](https://github.com/osrg/gobgp), [RustyBGP](https://github.com/osrg/rustybgp), [Frr](https://github.com/FRRouting/frr), and [Bird](https://gitlab.nic.cz/labs/bird). I use this tool for benchmarking RustyBGP. Please feel free to create a pull request for new features (such as adding other BGP software support).

## How works

```bash
$ sudo ./target/debug/update-watcher 
elasped: 1.272783ms, peers: 0, stabilized: false
elasped: 1.003315706s, peers: 0, stabilized: false
elasped: 2.005408534s, peers: 1, stabilized: false
elasped: 3.007413927s, peers: 1, stabilized: false
elasped: 4.009606323s, peers: 3, stabilized: false
elasped: 5.011790022s, peers: 3, stabilized: false
elasped: 6.014099694s, peers: 4, stabilized: false
elasped: 7.016293353s, peers: 4, stabilized: false
elasped: 8.019855141s, peers: 4, stabilized: true
elasped: 9.022806913s, peers: 4, stabilized: true
```

Firstly, the tool deletes the iptables rule to drop BGP packets. Then it queries the BGP daemon running locally every second about the number of update messages to be sent and received. If the three consecutive results are same, the `stabilized` message becomes true.

The way for query is parsing the output of command line tools for Frr and Bird, the GRPC APIs for GoBGP and RustyBGP.

## Typical benchmark setup

![](https://github.com/fujita/misc/raw/master/.github/assets/update-watcher.png)

I use multiple EC2 instances; one for this update-watcher and the BGP daemon to be benchmarked, others for BGP peers with full routes.

### Peer instances

I run multiple GoBGP daemons on one EC2 instance. Each GoBGP daemon has the full routes. The trick is running an EC2 instance with multiple IP addresses and assigning an IP address and one GoBGP daemon (use `local-address` option in config file). You also need assign unique gRPC listen port to each GoBGP daemon (use `api-hosts` command line option). There are some other stuff to be configured. Check out [an example script](https://github.com/fujita/misc/tree/master/fullroute-bench/config/cdk-peer.sh) that I use with [AWS CDK](https://aws.amazon.com/jp/cdk/).

I use [the slightly modified version of GoBGP](https://github.com/fujita/gobgp/releases/download/injector/gobgp_SNAPSHOT-a0615824_linux_arm64.tar.gz) for peers. It simply drops all routes to be received (which the target BGP daemon advertises). It hugely saves memory and CPU usage.

### Targeted instance

Firstly, let's create an iptables rule to drop all BGP packets to make sure that the peers don't connect to the targeted BGP daemon:

```bash
$ sudo iptables -A INPUT -p tcp --dport 179 -j DROP
```

`update-watcher` automatically removes the rule when it starts.

Needs to set up the targeted BGP daemon that accepts any peers. The config file examples for Frr and Bird can be found [here](https://github.com/fujita/misc/tree/master/fullroute-bench/config). It's easier in case of RustyBGP:

```bash
$ sudo rustybgpd --any-peers --as-number 65001 --router-id 1.1.1.1
```

Now ready to run `update-watcher`.
