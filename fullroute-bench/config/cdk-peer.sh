#!/bin/sh

# Target BGP daemon address
REMOTE="172.30.36.250"

declare -a ADDRS=($(ip addr show eth0 | sed -nEe 's/^[ \t]*inet[ \t]*([0-9.]+)\/.*$/\1/p'))

cd /root
curl -OL https://github.com/fujita/gobgp/releases/download/injector/gobgp_SNAPSHOT-a0615824_linux_arm64.tar.gz
tar xzf gobgp_SNAPSHOT-a0615824_linux_arm64.tar.gz
curl -OL http://archive.routeviews.org/route-views.wide/bgpdata/2019.08/RIBS/rib.20190830.0800.bz2
bzip2 -d rib.20190830.0800.bz2
cp gobgpd gobgp /usr/bin

i=0
for ADDR in ${ADDRS[@]}; do
    AS=$(echo $ADDR|awk -F '.' '{print $1+$2+$3+$4}')
    PORT=$((50051+i))
    cat <<EOF > gobgpd.conf-${ADDR}
    [global.config]
    as = ${AS}
    router-id = "${ADDR}"
    port=-1
    [[neighbors]]
    [neighbors.config]
    peer-as = 65001
    neighbor-address = "${REMOTE}"
    [neighbors.transport.config]
    local-address = "${ADDR}"
    [neighbors.timers.config]
    hold-time = 180
    connect-retry = 1
    idle-hold-time-after-reset = 5
EOF
    /usr/bin/gobgpd -f gobgpd.conf-${ADDR} --api-hosts=${ADDR}:50051 2>&1 > log-${ADDR} &
    sleep 5
    /usr/bin/gobgp --host ${ADDR} n ${REMOTE} disable
    /usr/bin/gobgp --host ${ADDR} mrt inject global --no-ipv6 --nexthop ${ADDR} --only-best rib.20190830.0800
    let i++
done

i=0
for ADDR in ${ADDRS[@]}; do
    /usr/bin/gobgp --host ${ADDR} n ${REMOTE} enable
    let i++
done
