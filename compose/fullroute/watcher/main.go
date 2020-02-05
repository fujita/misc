package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	api "github.com/osrg/gobgp/api"
	"google.golang.org/grpc"
)

var (
	peerOpt  = flag.Int("p", 8, "number of peers")
	routeOpt = flag.Int64("r", 771684, "number of routes")
	startBgp = flag.Bool("start-bgp", false, "start bgp")
)

func main() {
	flag.Parse()

	nr := uint64(*peerOpt) * uint64(*routeOpt)
	fmt.Println("wait for ", *peerOpt, " peers; each has ", *routeOpt, " routes.")

	grpcOpts := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}

	conn, err := grpc.DialContext(context.Background(), "127.0.0.1:50051", grpcOpts...)
	if err != nil {
		fmt.Println("can't connect ", err)
		os.Exit(0)
	}

	client := api.NewGobgpApiClient(conn)

	if *startBgp == true {
		_, err := client.StartBgp(context.Background(),
			&api.StartBgpRequest{
				Global: &api.Global{
					As:       65001,
					RouterId: "1.1.1.1",
				},
			})
		if err != nil {
			fmt.Println("failed to start bgp ", err)
			os.Exit(1)
		}
	}
	init := false
	neighbors := []string{}
	var start time.Time
	for {
		stream, err := client.ListPeer(context.Background(), &api.ListPeerRequest{})
		if err != nil {
			fmt.Println("failed to ListPeer ", err)
			os.Exit(0)
		}

		peers := 0
		accepted := uint64(0)
		neighbors = []string{}
		for {
			r, err := stream.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Println("failed to parse the response ", err)
				os.Exit(0)
			}
			if init == false {
				init = true
				start = time.Now()
			}
			peers++

			for _, afisafi := range r.Peer.GetAfiSafis() {
				accepted += afisafi.State.Accepted
			}
			neighbors = append(neighbors, r.Peer.GetState().GetNeighborAddress())
		}
		if init {
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " ", peers, " peers ", accepted, " accepted")
		}
		if accepted == nr {
			break
		}
		time.Sleep(time.Second)
	}
	fmt.Println("receiving finished: ", time.Since(start).Seconds(), " seconds")

	clients := []api.GobgpApiClient{}
	for _, n := range neighbors {
		conn, err = grpc.DialContext(context.Background(), fmt.Sprintf("%s:50051", n), grpcOpts...)
		if err != nil {
			fmt.Printf("can't connect %s %v", n, err)
			os.Exit(0)
		}
		clients = append(clients, api.NewGobgpApiClient(conn))
	}

	old := uint64(0)
	for {
		n := uint64(0)
		for i, client := range clients {
			rsp, err := client.GetTable(context.Background(), &api.GetTableRequest{
				Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}})
			if err != nil {
				fmt.Printf("failed to ListPeer %s %v", neighbors[i], err)
				os.Exit(0)
			}
			n += rsp.GetNumPath()
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " ", n, " paths")
		if n == old {
			break
		}
		time.Sleep(time.Second * 1)
		old = n
	}

	fmt.Println("finished: ", time.Since(start).Seconds(), " seconds")
}
