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

struct Bird {
    re1: Regex,
    re2: Regex,
    re3: Regex,
}

impl Bird {
    fn new() -> Self {
        Bird {
            re1: Regex::new(r"Neighbor address:\D+([\d\.]+)").unwrap(),
            re2: Regex::new(r"Import updates:\D+([\d\.]+)").unwrap(),
            re3: Regex::new(r"Export updates:\D+([\d\.]+)").unwrap(),
        }
    }

    fn get_counter(&self) -> Counter {
        let mut m = Counter {
            inner: HashMap::new(),
        };
        let output = Command::new("birdc")
            .args(&["show", "protocols", "all"])
            .output()
            .expect("failed to execute birdc command");
        let lines = str::from_utf8(&output.stdout).unwrap().lines();
        let mut addr: Option<Ipv4Addr> = None;
        let mut rx = 0;
        for s in lines {
            if let Some(caps) = self.re1.captures(s) {
                addr = Some(Ipv4Addr::from_str(caps.get(1).unwrap().as_str()).unwrap());
            } else if let Some(caps) = self.re2.captures(s) {
                rx = caps.get(1).unwrap().as_str().parse::<u64>().unwrap();
            } else if let Some(caps) = self.re3.captures(s) {
                let peer_addr = addr.take().unwrap();
                if rx != 0 {
                    m.inner.insert(
                        peer_addr,
                        PeerCounter {
                            tx: caps.get(1).unwrap().as_str().parse::<u64>().unwrap(),
                            rx,
                        },
                    );
                }
                rx = 0;
            }
        }
        m
    }
}

struct OpenBgpd {}

impl OpenBgpd {
    fn new() -> Self {
        OpenBgpd {}
    }

    fn get_counter(&self) -> Counter {
        let mut m = Counter {
            inner: HashMap::new(),
        };
        let output = Command::new("bgpctl")
            .args(&["-j", "show", "neighbor"])
            .output()
            .expect("failed to execute process");

        let j: serde_json::Value =
            serde_json::from_str(str::from_utf8(&output.stdout).unwrap()).unwrap();

        for n in j["neighbors"].as_array().unwrap() {
            let addr = n["remote_addr"].as_str().unwrap();
            // skip "172.0.0.0/8" configuration
            if addr.to_string().contains("/") {
                continue;
            }

            let tx = n["stats"]["message"]["sent"]["updates"].as_i64().unwrap() as u64;
            let rx = n["stats"]["message"]["received"]["updates"]
                .as_i64()
                .unwrap() as u64;

            m.inner
                .insert(Ipv4Addr::from_str(addr).unwrap(), PeerCounter { tx, rx });
        }

        m
    }
}

struct Target {
    gobgp: Option<GoBgp>,
    frr: Option<Frr>,
    bird: Option<Bird>,
    openbgpd: Option<OpenBgpd>,
}

impl Target {
    async fn get_counter(&mut self) -> Counter {
        if let Some(frr) = self.frr.as_ref() {
            return frr.get_counter();
        } else if let Some(bird) = self.bird.as_mut() {
            return bird.get_counter();
        } else if let Some(openbgpd) = self.openbgpd.as_mut() {
            return openbgpd.get_counter();
        } else if let Some(gobgp) = self.gobgp.as_mut() {
            return gobgp.get_counter().await;
        }
        panic!("");
    }
}

// Needs to block bgp packet before staring this program
// # iptables -A INPUT -p tcp --dport 179 -j DROP
fn start_bgp() {
    let output = Command::new("sudo")
        .args(&[
            "iptables", "-D", "INPUT", "-p", "tcp", "--dport", "179", "-j", "DROP",
        ])
        .output()
        .expect("failed to execute process");
    if !output.status.success() {
        std::io::stderr().write_all(&output.stderr).unwrap();
        std::process::exit(1);
    }
}

fn is_stabilized(history: &Vec<Counter>) -> bool {
    for i in 1..history.len() {
        if history[0] != history[i] {
            return false;
        }
    }
    true
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = App::new("update-watcher")
        .arg(
            Arg::with_name("target")
                .long("target")
                .takes_value(true)
                .help("Sets target (ffr|bird|openbgp)"),
        )
        .get_matches();

    let mut target = if let Some(t) = args.value_of("target") {
        match t {
            "ffr" => Target {
                gobgp: None,
                frr: Some(Frr::new()),
                bird: None,
                openbgpd: None,
            },
            "bird" => Target {
                gobgp: None,
                frr: None,
                bird: Some(Bird::new()),
                openbgpd: None,
            },
            "openbgp" => Target {
                gobgp: None,
                frr: None,
                bird: None,
                openbgpd: Some(OpenBgpd::new()),
            },
            _ => {
                println!("supported target: bird or ffr or openbgp");
                return Ok(());
            }
        }
    } else {
        let client = api::gobgp_api_client::GobgpApiClient::connect("http://0.0.0.0:50051")
            .await
            .unwrap();
        Target {
            gobgp: Some(GoBgp { client }),
            frr: None,
            bird: None,
            openbgpd: None,
        }
    };

    let mut stats = Vec::new();
    let num_stats = 3;

    start_bgp();
    let start_time = tokio::time::Instant::now();
    loop {
        let m = target.get_counter().await;
        let num_peers = m.inner.len();

        stats.insert(0, m);
        while stats.len() > num_stats {
            stats.pop();
        }

        let finished = if stats.len() == num_stats && num_peers > 0 {
            is_stabilized(&stats)
        } else {
            false
        };

        println!(
            "elasped: {:?}, peers: {}, stabilized: {}",
            start_time.elapsed(),
            num_peers,
            finished,
        );

        tokio::time::sleep(tokio::time::Duration::from_secs(1)).await;
    }
}
