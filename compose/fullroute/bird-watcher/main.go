package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	api "github.com/osrg/gobgp/api"
	"google.golang.org/grpc"
)

const cmd = "/usr/local/bin/birdc"

var (
	peerOpt  = flag.Int("p", 8, "number of peers")
	routeOpt = flag.Int64("r", 771684, "number of routes")
)

func main() {
	flag.Parse()

	nr := uint64(*peerOpt) * uint64(*routeOpt)
	fmt.Println("wait for ", *peerOpt, " peers; each has ", *routeOpt, " routes.")

	var start time.Time
	r := regexp.MustCompile(`\w+\S+`)
PEER_LABEL:
	for {
		args := []string{"show", "protocols"}
		out, err := exec.Command(cmd, args...).Output()
		if err != nil {
			fmt.Println("failed to exec ", err)
			time.Sleep(time.Millisecond * 500)
			continue
		}

		for _, s := range strings.Split(string(out), "\n")[2:] {
			l := r.FindAllString(s, -1)
			if len(l) > 4 {
				if l[4] == "Established" {
					start = time.Now()
					fmt.Println(start.Format("2006/01/02 15:04:05"), "the benchmark started")
					break PEER_LABEL
				}
			}
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), "waiting for peer")
		time.Sleep(time.Second)
	}

	r = regexp.MustCompile(`\d+`)
	for paths := uint64(0); ; {
		args := []string{"show", "route", "count"}
		out, err := exec.Command(cmd, args...).Output()
		if err != nil {
			fmt.Println("failed to exec ", err)
			os.Exit(0)
		}

		s := strings.Split(string(out), "\n")[1]
		l, err := strconv.ParseUint(r.FindAllString(s, -1)[0], 10, 64)
		if l < paths {
			fmt.Println(time.Now().Format("2006/01/02 15:04:05"), "the number of paths decreased!")
		}
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), l, "paths", time.Since(start).Seconds(), "secs")

		if l == nr {
			break
		}
		paths = l
		time.Sleep(time.Second * 2)
	}
	fmt.Println("receiving finished:", time.Since(start).Seconds(), "secs")

	args := []string{"show", "protocols", "all"}
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		fmt.Println("failed to exec ", err)
		os.Exit(0)
	}

	r = regexp.MustCompile(`Neighbor address: ((\w|.)+)`)
	peers := []string{}
	for _, s := range strings.Split(string(out), "\n") {
		l := r.FindStringSubmatch(s)
		if len(l) > 0 {
			peers = append(peers, l[1])
		}
	}

	grpcOpts := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}
	clients := []api.GobgpApiClient{}
	for _, addr := range peers {
		conn, err := grpc.DialContext(context.Background(), fmt.Sprintf("%s:50051", addr), grpcOpts...)
		if err != nil {
			fmt.Printf("can't connect %s %v", addr, err)
			os.Exit(0)
		}
		clients = append(clients, api.NewGobgpApiClient(conn))
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
		fmt.Println(time.Now().Format("2006/01/02 15:04:05"), n, "paths")
		if n == old {
			break
		}
		time.Sleep(time.Second * 1)
		old = n
	}

	fmt.Println("finished:", time.Since(start).Seconds(), "secs")
}
