use clap::{App, Arg};
use regex::Regex;
use std::collections::HashMap;
use std::io::Write;
use std::net::Ipv4Addr;
use std::process::Command;
use std::str;
use std::str::FromStr;

mod api {
    tonic::include_proto!("gobgpapi");
}

#[derive(Debug)]
struct Counter {
    inner: HashMap<Ipv4Addr, PeerCounter>,
}

impl PartialEq for Counter {
    fn eq(&self, other: &Self) -> bool {
        if self.inner.len() != other.inner.len() {
            return false;
        }
        for k in other.inner.keys() {
            match self.inner.get(k) {
                Some(s) => {
                    if s.tx != other.inner.get(k).unwrap().tx
                        || s.rx != other.inner.get(k).unwrap().rx
                    {
                        return false;
                    }
                }
                None => return false,
            }
        }
        true
    }
}

#[derive(Debug)]
struct PeerCounter {
    tx: u64,
    rx: u64,
}

struct GoBgp {
    client: api::gobgp_api_client::GobgpApiClient<tonic::transport::Channel>,
}

impl GoBgp {
    async fn get_counter(&mut self) -> Counter {
        let mut m = Counter {
            inner: HashMap::new(),
        };
        let mut rsp = self
            .client
            .list_peer(api::ListPeerRequest {
                address: "".to_string(),
                enable_advertised: false,
            })
            .await
            .unwrap()
            .into_inner();

        while let Some(mut peer) = rsp.message().await.unwrap() {
            let mut peer = peer.peer.take().unwrap();
            let mut state = peer.state.take().unwrap();
            let messages = state.messages.take().unwrap();
            m.inner.insert(
                Ipv4Addr::from_str(&state.neighbor_address).unwrap(),
                PeerCounter {
                    tx: messages.sent.unwrap().update,
                    rx: messages.received.unwrap().update,
                },
            );
        }
        m
    }
}

struct Frr {
    re1: Regex,
    re2: Regex,
}

impl Frr {
    fn new() -> Self {
        Frr {
            re1: Regex::new(r"^BGP neighbor\D+([\d\.]+)").unwrap(),
            re2: Regex::new(r"Updates:\D+(\d+)\D+(\d+)").unwrap(),
        }
    }

    fn get_counter(&self) -> Counter {
        let mut m = Counter {
            inner: HashMap::new(),
        };

        // needs to add yourself to vtysh group for capability to run vtysh
        let output = Command::new("vtysh")
            .arg("-c")
            .arg("show bgp neighbors")
            .output()
            .expect("failed to execute process");

        let lines = str::from_utf8(&output.stdout).unwrap().lines();
        let mut addr: Option<Ipv4Addr> = None;
        for s in lines {
            if let Some(caps) = self.re1.captures(s) {
                assert_eq!(caps.len(), 2);
                assert_eq!(addr.is_none(), true);
                addr = Some(Ipv4Addr::from_str(caps.get(1).unwrap().as_str()).unwrap());
            }
            if let Some(caps) = self.re2.captures(s) {
                assert_eq!(caps.len(), 3);
                assert_eq!(addr.is_some(), true);
                m.inner.insert(
                    addr.take().unwrap(),
                    PeerCounter {
                        tx: caps.get(1).unwrap().as_str().parse::<u64>().unwrap(),
                        rx: caps.get(2).unwrap().as_str().parse::<u64>().unwrap(),
                    },
                );
            }
        }
        m
    }
}

struct Target {
    frr: Option<Frr>,
    gobgp: Option<GoBgp>,
}

impl Target {
    async fn get_counter(&mut self) -> Counter {
        if let Some(frr) = self.frr.as_ref() {
            return frr.get_counter();
        } else if let Some(gobgp) = self.gobgp.as_mut() {
            return gobgp.get_counter().await;
        }
        panic!("");
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = App::new("update-watcher")
        .arg(Arg::with_name("frr").long("frr").help("use frr"))
        .get_matches();

    let mut target = if args.is_present("frr") {
        Target {
            frr: Some(Frr::new()),
            gobgp: None,
        }
    } else {
        let client = api::gobgp_api_client::GobgpApiClient::connect("http://0.0.0.0:50051")
            .await
            .unwrap();

        Target {
            frr: None,
            gobgp: Some(GoBgp { client }),
        }
    };

    let mut stats = Vec::new();
    let num_stats = 3;

    // before run this code, needs to block bgp packet
    // iptables -A INPUT -p tcp --dport 179 -j DROP
    let output = Command::new("sudo")
        .arg("iptables")
        .arg("-D")
        .arg("INPUT")
        .arg("-p")
        .arg("tcp")
        .arg("--dport")
        .arg("179")
        .arg("-j")
        .arg("DROP")
        .output()
        .expect("failed to execute process");
    if !output.status.success() {
        std::io::stderr().write_all(&output.stderr).unwrap();
        std::process::exit(1);
    }

    let start_time = tokio::time::Instant::now();
    loop {
        let m = target.get_counter().await;
        println!("peer numbers: {:?}", m.inner.len());
        stats.insert(0, m);
        while stats.len() > num_stats {
            stats.pop();
        }

        let mut finished = true;
        if stats.len() == num_stats {
            for i in 1..stats.len() {
                if stats[0] != stats[i] {
                    finished = false;
                }
            }
        } else {
            finished = false;
        }
        println!(
            "elasped: {:?}, finished: {}",
            start_time.elapsed(),
            finished
        );

        tokio::time::sleep(tokio::time::Duration::from_secs(1)).await;
    }
}
