package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/k-sone/critbitgo"
	"github.com/osrg/gobgp/pkg/packet/bgp"
	"github.com/osrg/gobgp/pkg/packet/mrt"
)

var (
	prefixes  []bgp.AddrPrefixInterface
	intMap    map[uint64]bgp.AddrPrefixInterface
	stringMap map[string]bgp.AddrPrefixInterface
	ir        *iradix.Tree
	cri       *critbitgo.Trie
)

func stringKey(nlri bgp.AddrPrefixInterface) string {
	switch T := nlri.(type) {
	case *bgp.IPAddrPrefix:
		b := make([]byte, 5)
		copy(b, T.Prefix.To4())
		b[4] = T.Length
		return *(*string)(unsafe.Pointer(&b))
	case *bgp.IPv6AddrPrefix:
		b := make([]byte, 17)
		copy(b, T.Prefix.To16())
		b[16] = T.Length
		return *(*string)(unsafe.Pointer(&b))
	}
	return nlri.String()
}

func insertStringKey(b *benchmark) {
	m := make(map[string]bgp.AddrPrefixInterface)
	for i, v := range prefixes {
		m[stringKey(v)] = prefixes[i]
	}
	stringMap = m
}

func lookupStringKey(b *benchmark) {
	for i := 0; i < b.n; i++ {
		_ = stringMap[stringKey(prefixes[i])]
	}
}

func intKey(nlri bgp.AddrPrefixInterface) uint64 {
	switch T := nlri.(type) {
	case *bgp.IPAddrPrefix:
		return uint64(T.Length)<<32 | uint64(binary.BigEndian.Uint32(T.Prefix))
	}
	return 0
}

func insertIntKey(b *benchmark) {
	m := make(map[uint64]bgp.AddrPrefixInterface)
	for i, v := range prefixes {
		m[intKey(v)] = prefixes[i]
	}
	intMap = m
}

func lookupIntKey(b *benchmark) {
	for i := 0; i < b.n; i++ {
		_ = intMap[intKey(prefixes[i])]
	}
}

func radixKey(nlri bgp.AddrPrefixInterface) []byte {
	switch T := nlri.(type) {
	case *bgp.IPAddrPrefix:
		return append(T.Prefix, byte(T.Length))
	}
	return []byte{}
}

func insertRadix(b *benchmark) {
	r := iradix.New()
	ir = r
	for i, v := range prefixes {
		r, _, _ = r.Insert(radixKey(v), prefixes[i])
	}
}

func lookupRadix(b *benchmark) {
	r := ir
	for i := 0; i < b.n; i++ {
		_, _ = r.Get(radixKey(prefixes[i]))
	}
}

func insertCritbit(b *benchmark) {
	t := critbitgo.NewTrie()
	cri = t
	for i, v := range prefixes {
		t.Insert(radixKey(v), prefixes[i])
	}
}

func lookupCritbit(b *benchmark) {
	t := cri
	for i := 0; i < b.n; i++ {
		_, _ = t.Get(radixKey(prefixes[i]))
	}
}

var name = flag.String("f", "hello", "mrt filename")

type benchmark struct {
	n        int
	start    time.Time
	duration time.Duration
}

func (b *benchmark) reset() {
	b.start = time.Now()
}

func (b *benchmark) stop() {
	b.duration = time.Since(b.start)
}

func (b *benchmark) run(name string, f func(b *benchmark)) {
	b.reset()
	f(b)
	b.stop()
	fmt.Println(name, " ", float64(b.duration.Nanoseconds())/float64(b.n), " ns/op")
}

func main() {
	flag.Parse()

	fp, err := os.Open(*name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	prefixes = make([]bgp.AddrPrefixInterface, 0)

	scanner := bufio.NewScanner(fp)
	scanner.Split(mrt.SplitMrt)

	for scanner.Scan() {
		b := scanner.Bytes()
		hdr := &mrt.MRTHeader{}
		err := hdr.DecodeFromBytes(b)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		var msg *mrt.MRTMessage
		msg, err = mrt.ParseMRTBody(hdr, b[mrt.MRT_COMMON_HEADER_LEN:])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if hdr.Type != mrt.TABLE_DUMPv2 {
			continue
		}

		switch mrt.MRTSubTypeTableDumpv2(hdr.SubType) {
		case mrt.RIB_IPV4_UNICAST:
			rib := msg.Body.(*mrt.Rib)
			prefixes = append(prefixes, rib.Prefix)
		}
	}
	fmt.Println("prefixes = ", len(prefixes))
	b := benchmark{
		n: len(prefixes),
	}

	b.run("stringMap", insertStringKey)
	b.run("intMap", insertIntKey)
	b.run("iradix", insertRadix)
	b.run("cribit", insertCritbit)
	b.run("intMap lookup", lookupIntKey)
	b.run("stringMap lookup", lookupStringKey)
	b.run("radix lookup", lookupRadix)
	b.run("cribit lookup", lookupCritbit)
}
