```
$ go run rib.go -f ~/rib.20190802.1000 
INSERT
string key map insert          509.00898 ns/op
int key map insert             266.19737 ns/op
mutable radix insert          1619.38138 ns/op
immutable radix insert        6619.35501 ns/op
critbit insert                 418.73917 ns/op
LOOKUP
string key map lookup          311.91870 ns/op
int key map lookup             160.57204 ns/op
mutable radix lookup          1483.49785 ns/op
immutable radix lookup         455.51795 ns/op
critbit lookup                 202.81668 ns/op
WALK
string key map walk             17.52351 ns/op
int key map walk                16.82702 ns/op
mutable radix walk             104.49105 ns/op
immutable radix walk            95.97988 ns/op
critbit walk                    60.48976 ns/op
DELETE
string key map delete          311.92989 ns/op
int key map delete             172.79628 ns/op
mutable radix delete          1216.13589 ns/op
immutable radix delete        5412.09430 ns/op
critbit walk                   179.78286 ns/op

the number of prefixes =  767235
```

```
$ head /proc/cpuinfo 
processor	     : 0
vendor_id	     : GenuineIntel
cpu family	     : 6
model		       : 85
model name	       : Intel(R) Xeon(R) Gold 5118 CPU @ 2.30GHz
stepping	       : 4
microcode	       : 0x2000050
cpu MHz		       	 : 1000.826
cache size		 : 16896 KB
physical id		 : 0
```

