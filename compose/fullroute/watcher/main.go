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
	peerOpt   = flag.Int("p", 8, "number of peers")
	routeOpt  = flag.Int64("r", 771684, "number of routes")
	startBgp  = flag.Bool("start-bgp", false, "start bgp")
	policyOpt = flag.Bool("add-policy", false, "add policies")
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

	if *policyOpt == true {
		num := 1000

		for i := 0; i < num; i++ {
			_, err = client.AddDefinedSet(context.Background(), &api.AddDefinedSetRequest{
				DefinedSet: &api.DefinedSet{
					DefinedType: api.DefinedType(3),
					Name:        fmt.Sprintf("d%d", i),
					List:        []string{fmt.Sprintf("^%d_", i+65000)},
				},
			})
			if err != nil {
				fmt.Println("failed to add AddDefinedSet() ", err)
				os.Exit(1)
			}
			_, err = client.AddStatement(context.Background(), &api.AddStatementRequest{
				Statement: &api.Statement{
					Name: fmt.Sprintf("s%d", i),
					Conditions: &api.Conditions{
						AsPathSet: &api.MatchSet{
							Name: fmt.Sprintf("d%d", i),
						},
					},
				},
			})
			if err != nil {
				fmt.Println("failed to add AddStatement() ", err)
				os.Exit(1)
			}
			_, err = client.AddPolicy(context.Background(), &api.AddPolicyRequest{
				Policy: &api.Policy{
					Name: fmt.Sprintf("p%d", i),
					Statements: []*api.Statement{&api.Statement{
						Name: fmt.Sprintf("s%d", i),
					}},
				},
				ReferExistingStatements: true,
			})
			if err != nil {
				fmt.Println("failed to add AddPolicy() ", err)
				os.Exit(1)
			}
		}
		var p []*api.Policy
		for i := 0; i < num; i++ {
			p = append(p, &api.Policy{Name: fmt.Sprintf("p%d", i)})
		}
		_, err = client.AddPolicyAssignment(context.Background(), &api.AddPolicyAssignmentRequest{
			Assignment: &api.PolicyAssignment{
				Name:      "global",
				Direction: api.PolicyDirection(1),
				Policies:  p,
			},
		})
	}

	var start time.Time
PEER_LABEL:
	for {
		stream, err := client.ListPeer(context.Background(), &api.ListPeerRequest{})
		if err != nil {
			fmt.Println("failed to ListPeer ", err)
			os.Exit(0)
		}
		for {
			r, err := stream.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Println("failed to parse the response ", err)
				os.Exit(0)
			}
			// all are dynamic peers. So when we have one peer, we think that the benchmark started.
			start = time.Now()
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), r.Peer.GetState().GetNeighborAddress(), " connected")
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " the benchmark started ", r.Peer.GetState().GetNeighborAddress())
			break PEER_LABEL
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " waiting for peer")
		time.Sleep(time.Second)
	}

	for paths := uint64(0); ; {
		rsp, err := client.GetTable(context.Background(), &api.GetTableRequest{
			Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}})
		if err != nil {
			fmt.Printf("failed to GetTable %v", err)
			os.Exit(0)
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), rsp.GetNumPath(), " paths")
		if rsp.GetNumPath() == nr {
			break
		}
		if rsp.GetNumPath() < paths {
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), " the number of paths decreased!")
		}
		paths = rsp.GetNumPath()
		time.Sleep(time.Second * 2)
	}
	fmt.Println("receiving finished: ", time.Since(start).Seconds(), " seconds")

	clients := []api.GobgpApiClient{}
	if stream, err := client.ListPeer(context.Background(), &api.ListPeerRequest{}); err != nil {
		fmt.Println("failed to ListPeer ", err)
		os.Exit(0)
	} else {
		for {
			r, err := stream.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				fmt.Println("failed to parse the response ", err)
				os.Exit(0)
			}
			conn, err = grpc.DialContext(context.Background(), fmt.Sprintf("%s:50051", r.Peer.GetState().GetNeighborAddress()), grpcOpts...)
			if err != nil {
				fmt.Printf("can't connect %s %v", r.Peer.GetState().GetNeighborAddress(), err)
				os.Exit(0)
			}
			clients = append(clients, api.NewGobgpApiClient(conn))
		}
	}

	for old := uint64(0); ; {
		n := uint64(0)
		for _, client := range clients {
			rsp, err := client.GetTable(context.Background(), &api.GetTableRequest{
				Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}})
			if err != nil {
				fmt.Printf("failed to GetTable %v", err)
				os.Exit(0)
			}
			n += rsp.GetNumPath()
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), n, " paths")
		if n == old {
			break
		}
		time.Sleep(time.Second * 1)
		old = n
	}

	fmt.Println("finished: ", time.Since(start).Seconds(), " seconds")
}
