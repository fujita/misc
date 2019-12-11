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

	init := false
	var start time.Time
	for {
		stream, err := client.ListPeer(context.Background(), &api.ListPeerRequest{})
		if err != nil {
			fmt.Println("failed to ListPeer ", err)
			os.Exit(0)
		}

		peers := 0
		accepted := uint64(0)
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
		}
		if init {
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " ", peers, " peers ", accepted, " accepted")
		}
		if accepted == nr {
			break
		}
		time.Sleep(time.Second)
	}
	fmt.Println("finished: ", time.Since(start).Seconds(), " seconds")
}
