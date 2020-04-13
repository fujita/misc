#! /bin/sh

case "$1" in
    build)
	BRANCH=master
	if [ $# -eq 2 ]; then
	       BRANCH=$2
	fi
	       
	cat <<EOF > Dockerfile
FROM rust as builder

ENV HOME /root
WORKDIR /root

RUN curl -OL https://github.com/osrg/gobgp/releases/download/v2.11.0/gobgp_2.11.0_linux_amd64.tar.gz
RUN tar xzf gobgp_2.11.0_linux_amd64.tar.gz

RUN apt-get update && \
    apt-get install -y \
        build-essential \
        cmake \
        curl \
        musl-dev \
        musl-tools

RUN rustup target add x86_64-unknown-linux-musl
RUN rustup component add rustfmt
RUN git clone https://github.com/fujita/rustybgp.git
RUN cd rustybgp && git checkout -b build origin/$BRANCH
RUN cd rustybgp && cargo build --release --target x86_64-unknown-linux-musl

FROM alpine
WORKDIR /root
COPY --from=builder /root/rustybgp/target/x86_64-unknown-linux-musl/release/daemon /usr/sbin/rustybgpd
COPY --from=builder /root/gobgpd /usr/sbin
COPY --from=builder /root/gobgp /usr/sbin
EOF

	docker build -t tomo/rustybgp  --no-cache  .
	docker-compose -f docker-compose-rusty.yml build  --no-cache server
    ;;

    *)
	echo "Usage: {build}" >&2
	exit 1
    ;;
esac

exit 0
	    
