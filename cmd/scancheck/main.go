package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"new_era_go/internal/discovery"
)

func main() {
	printLocalInterfaces()
	fmt.Println("")

	opts := discovery.DefaultOptions()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	start := time.Now()
	candidates, err := discovery.Scan(ctx, opts)
	dur := time.Since(start)

	fmt.Printf("scan duration: %s\n", dur.Round(time.Millisecond))
	if err != nil {
		fmt.Printf("scan error: %v\n", err)
	}
	fmt.Printf("candidates: %d\n", len(candidates))

	for i, candidate := range candidates {
		fmt.Printf("%2d) %s:%d verified=%v addr=0x%02X proto=%s score=%d reason=%s banner=%q\n",
			i+1,
			candidate.Host,
			candidate.Port,
			candidate.Verified,
			candidate.ReaderAddress,
			candidate.Protocol,
			candidate.Score,
			candidate.Reason,
			candidate.Banner,
		)
	}
}

func printLocalInterfaces() {
	fmt.Println("local interfaces:")
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				fmt.Printf("  - %s : %s\n", iface.Name, ipNet.String())
			}
		}
	}
}
