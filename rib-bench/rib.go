package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
	"unsafe"

	"github.com/armon/go-radix"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/k-sone/critbitgo"
	"github.com/osrg/gobgp/pkg/packet/bgp"
	"github.com/osrg/gobgp/pkg/packet/mrt"
)

var (
	prefixes  []bgp.AddrPrefixInterface
	intMap    map[uint64]bgp.AddrPrefixInterface
	stringMap map[string]bgp.AddrPrefixInterface
	ir        *iradix.Tree // immutable
	mr        *radix.Tree  // mutable
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

func deleteStringKey(b *benchmark) {
	m := stringMap
	for i := 0; i < b.n; i++ {
		delete(m, stringKey(prefixes[i]))
	}
}

func lookupStringKey(b *benchmark) {
	m := stringMap
	for i := 0; i < b.n; i++ {
		_ = m[stringKey(prefixes[i])]
	}
}

func walkStringKey(b *benchmark) {
	m := stringMap
	for _, v := range m {
		_ = v
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

func deleteIntKey(b *benchmark) {
	m := intMap
	for i := 0; i < b.n; i++ {
		delete(m, intKey(prefixes[i]))
	}
}

func lookupIntKey(b *benchmark) {
	m := intMap
	for i := 0; i < b.n; i++ {
		_ = m[intKey(prefixes[i])]
	}
}

func walkIntKey(b *benchmark) {
	m := intMap
	for _, v := range m {
		_ = v
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
	for i, v := range prefixes {
		r, _, _ = r.Insert(radixKey(v), prefixes[i])
	}
	ir = r
}

func deleteRadix(b *benchmark) {
	for i := 0; i < b.n; i++ {
		ir, _, _ = ir.Delete(radixKey(prefixes[i]))
	}
}

func lookupRadix(b *benchmark) {
	r := ir
	for i := 0; i < b.n; i++ {
		_, _ = r.Get(radixKey(prefixes[i]))
	}
}

func walkRadix(b *benchmark) {
	r := ir
	r.Root().Walk(func(k []byte, v interface{}) bool {
		return false
	})
}

func insertCritbit(b *benchmark) {
	t := critbitgo.NewTrie()
	cri = t
	for i, v := range prefixes {
		t.Insert(radixKey(v), prefixes[i])
	}
}

func deleteCritbit(b *benchmark) {
	t := cri
	for i := 0; i < b.n; i++ {
		t.Delete(radixKey(prefixes[i]))
	}
}

func lookupCritbit(b *benchmark) {
	t := cri
	for i := 0; i < b.n; i++ {
		_, _ = t.Get(radixKey(prefixes[i]))
	}
}

func walkCritbit(b *benchmark) {
	t := cri
	t.Walk(nil, func(k []byte, v interface{}) bool {
		return true
	})
}

func radixStringkey(nlri bgp.AddrPrefixInterface) string {
	switch T := nlri.(type) {
	case *bgp.IPAddrPrefix:
		var buffer bytes.Buffer
		b := T.Prefix
		max := T.Length
		for i := 0; i < len(b) && i < int(max); i++ {
			fmt.Fprintf(&buffer, "%08b", b[i])
		}
		return buffer.String()[:max]
	}
	return ""
}

func insertMutableRadix(b *benchmark) {
	r := radix.New()
	mr = r
	for i, v := range prefixes {
		r.Insert(radixStringkey(v), prefixes[i])
	}
}

func deleteMutableRadix(b *benchmark) {
	r := mr
	for i := 0; i < b.n; i++ {
		r.Delete(radixStringkey(prefixes[i]))
	}
}

func lookupMutableRadix(b *benchmark) {
	r := mr
	for i := 0; i < b.n; i++ {
		_, _ = r.Get(radixStringkey(prefixes[i]))
	}
}

func walkMutableRadix(b *benchmark) {
	r := mr
	r.Walk(func(s string, v interface{}) bool {
		return false
	})
}

var (
	name    = flag.String("f", "hello", "mrt filename")
	count   = flag.Int("c", 1, "count")
	profile = flag.String("p", "", "memory profile")
)

type benchmark struct {
	n        int
	count    int
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
	d := float64(0)
	for i := 0; i < b.count; i++ {
		b.reset()
		f(b)
		b.stop()
		d += float64(b.duration.Nanoseconds()) / float64(b.n)
	}
	fmt.Printf("%-30s%10.5f ns/op\n", name, d/float64(b.count))
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
	b := benchmark{
		n:     len(prefixes),
		count: *count,
	}

	fmt.Println("INSERT")
	b.run("string key map insert", insertStringKey)
	b.run("int key map insert", insertIntKey)
	b.run("mutable radix insert", insertMutableRadix)
	b.run("immutable radix insert", insertRadix)
	b.run("critbit insert", insertCritbit)

	f := func(n string, v, expected int) {
		if v != expected {
			fmt.Println("size of", n, "is", v, "but", expected, "is expected")
			os.Exit(1)
		}
	}

	prefixLen := len(prefixes)
	f("string key map", len(stringMap), prefixLen)
	f("int key map", len(intMap), prefixLen)
	f("mutable", ir.Len(), prefixLen)
	f("immutable", mr.Len(), prefixLen)
	f("cri", cri.Size(), prefixLen)

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	fmt.Println("LOOKUP")
	b.run("string key map lookup", lookupStringKey)
	b.run("int key map lookup", lookupIntKey)
	b.run("mutable radix lookup", lookupMutableRadix)
	b.run("immutable radix lookup", lookupRadix)
	b.run("critbit lookup", lookupCritbit)

	fmt.Println("WALK")
	b.run("string key map walk", walkStringKey)
	b.run("int key map walk", walkIntKey)
	b.run("mutable radix walk", walkMutableRadix)
	b.run("immutable radix walk", walkRadix)
	b.run("critbit walk", walkCritbit)

	fmt.Println("DELETE")
	b.run("string key map delete", deleteStringKey)
	b.run("int key map delete", deleteIntKey)
	b.run("mutable radix delete", deleteMutableRadix)
	b.run("immutable radix delete", deleteRadix)
	b.run("critbit walk", deleteCritbit)

	f("string key map", len(stringMap), 0)
	f("int key map", len(intMap), 0)
	f("mutable", mr.Len(), 0)
	f("immutable", ir.Len(), 0)
	f("cri", cri.Size(), 0)

	fmt.Println("\nthe number of prefixes = ", prefixLen)
}
