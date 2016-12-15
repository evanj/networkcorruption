package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"./datautil"
	"./sonocheck"
)

const packetPrint = 60000  // print after receiving this many packets
const udpOverhead = 20 + 8 // IPv4 = 20 bytes; udp = 8 bytes
const mtu = 1500           // Ethernet standard
const maxPacketLength = mtu - udpOverhead

type packetTimer struct {
	periodStart time.Time
	packets     int
	bytes       int

	totalPackets int64
}

func (p *packetTimer) increment(bytes int) {
	if p.packets == 0 {
		p.periodStart = time.Now()
	}
	p.packets += 1
	p.totalPackets += 1
	p.bytes += bytes
	if p.packets%packetPrint == 0 {
		elapsed := time.Now().Sub(p.periodStart)
		kbPerS := float64(p.bytes) / 1024. / elapsed.Seconds()
		gbitsPerS := float64(p.bytes*8) / 1e9 / elapsed.Seconds()
		fmt.Printf("%d packets %d bytes in %.1f s = %.1f kB/s = %.3f Gbits/s\n",
			p.packets, p.bytes, elapsed.Seconds(), kbPerS, gbitsPerS)

		p.periodStart = time.Now()
		p.packets = 0
		p.bytes = 0
	}
}

func runClient(connectAddr string, config *configuration) error {
	sleepDuration := time.Second / time.Duration(config.packetsPerSecond)
	conn, err := net.Dial("udp", connectAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// attempt to disable checksums (let's pass through more errors!)
	// NOTE: This should only work on IPv4 sockets
	sonocheck.LogSetNoCheck(conn)

	errors := 0
	filler := datautil.NewRandomFiller(time.Now().UnixNano())
	timer := packetTimer{}
	outBuf := make([]byte, config.packetLength)
	inBuf := make([]byte, config.packetLength)
	for {
		filler.Fill(outBuf)
		n, err := conn.Write(outBuf)
		if err != nil {
			return err
		}
		if n != len(outBuf) {
			return fmt.Errorf("wrote %d bytes; expected %d bytes", n, len(outBuf))
		}
		timer.increment(len(outBuf))

		err = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err != nil {
			return err
		}
		n, err = conn.Read(inBuf)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				fmt.Printf("timeout on read\n")
				return nil
			}
			return err
		}
		if n != len(inBuf) {
			return fmt.Errorf("wrote %d bytes; expected %d bytes", n, len(inBuf))
		}

		mismatches := datautil.MatchWithErrors(inBuf, outBuf, false)
		if mismatches.HasErrors() {
			errors += 1
			fmt.Printf("%d errors in %d packets\n", errors, timer.totalPackets)
			for _, offset := range mismatches.Offsets() {
				if offset%16 != 7 {
					panic("special address offset!")
				}
				// if offset <= 23 { // 23, 55, 71, 87 also have appeared
				//  fmt.Printf("low offset: %d\n", offset)
				//  fmt.Println(hex.EncodeToString(outBuf))
				//  fmt.Println(hex.Dump(outBuf))
				//  panic("low offset")
				// }
				if outBuf[offset]&(^byte(0x04)) != inBuf[offset] {
					panic(fmt.Sprintf("different kind of corruption at %d 0x%02x != 0x%02x",
						offset, outBuf[offset], inBuf[offset]))
				}
			}

			if errors >= config.errorLimit {
				fmt.Printf("hit the error limit")
				return nil
			}
		}
		time.Sleep(sleepDuration)
	}
}

func runServer(listenPort int, config *configuration) error {
	listenAddr := net.UDPAddr{net.IP([]byte{0, 0, 0, 0}), listenPort, ""}
	conn, err := net.ListenUDP("udp", &listenAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	sonocheck.LogSetNoCheck(conn)

	buf := make([]byte, config.packetLength)
	timer := packetTimer{}
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		timer.increment(n)
		// fmt.Printf("received %d from %s\n", n, clientAddr.String()

		n, err = conn.WriteTo(buf[:n], clientAddr)
		if err != nil {
			return err
		}
	}
}

type configuration struct {
	packetsPerSecond int
	packetLength     int
	errorLimit       int
}

func main() {
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		buf := make([]byte, 1<<20)
		for {
			<-sigs
			runtime.Stack(buf, true)
			fmt.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf)
		}
	}()

	listenPort := flag.Int("listen", 0, "start server listening on this port")
	connectAddr := flag.String("connect", "", "start clinet sending to this addr/port")
	length := flag.Int("length", maxPacketLength, "length of packet")
	rate := flag.Int("rate", 1000, "packets per second to send")
	errorLimit := flag.Int("errorLimit", 10000, "max errors before sender exits")
	flag.Parse()

	if *length <= 0 || *length > maxPacketLength {
		fmt.Fprintf(os.Stderr, "ERROR: -length=%d; must be between 0 and %d\n",
			*length, maxPacketLength)
		os.Exit(1)
	}

	config := configuration{*rate, *length, *errorLimit}

	var err error
	if *listenPort != 0 {
		err = runServer(*listenPort, &config)
	} else if len(*connectAddr) != 0 {
		err = runClient(*connectAddr, &config)
	} else {
		fmt.Fprintln(os.Stderr, "Specify one of -connect or -listen")
		os.Exit(1)
		return
	}
	if err != nil {
		panic(err)
	}
}
