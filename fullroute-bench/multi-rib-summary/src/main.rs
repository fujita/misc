use std::net::IpAddr;
use std::str::FromStr;

mod api {
    tonic::include_proto!("gobgpapi");
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut local_client = api::gobgp_api_client::GobgpApiClient::connect("http://0.0.0.0:50051")
        .await
        .unwrap();

    let mut rsp = local_client
        .list_peer(api::ListPeerRequest {
            address: "".to_string(),
            enable_advertised: false,
        })
        .await
        .unwrap()
        .into_inner();

    let mut v = Vec::new();
    while let Some(mut peer) = rsp.message().await.unwrap() {
        let mut peer = peer.peer.take().unwrap();
        let state = peer.state.take().unwrap();
        let peer_addr = IpAddr::from_str(&state.neighbor_address).unwrap();
        let j = tokio::spawn(async move {
            let mut client = api::gobgp_api_client::GobgpApiClient::connect(format!(
                "http://{}:50051",
                peer_addr
            ))
            .await
            .unwrap();
            let rsp = client
                .get_table(api::GetTableRequest {
                    table_type: 0,
                    family: Some(api::Family { afi: 1, safi: 1 }),
                    name: "".to_string(),
                })
                .await
                .unwrap()
                .into_inner();
            println!("{}: {:?}", peer_addr, rsp);
        });
        v.push(j);
    }
    for j in v {
        let _ = j.await;
    }

    Ok(())
}
