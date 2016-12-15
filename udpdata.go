package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
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

func fillWithPattern(buf []byte, conf *config) {
	if conf.expectedMode == modeRandom {
		datautil.NewRandomFiller(int64(conf.randomSeed)).Fill(buf)
		if conf.orMask != 0 {
			for i, b := range buf {
				buf[i] = b | conf.orMask
			}
		}
	} else {
		for i, _ := range buf {
			switch conf.expectedMode {
			case modeSequence:
				buf[i] = byte(i)
			case modeFill:
				for i := range buf {
					buf[i] = conf.expectedBytes[i%len(conf.expectedBytes)]
				}
			default:
				panic("unsupported mode: " + strconv.Itoa(int(conf.expectedMode)))
			}
		}
	}
}

func runClient(connectAddr string, packetsPerSecond int, conf *config) error {
	sleepDuration := time.Second / time.Duration(packetsPerSecond)
	conn, err := net.Dial("udp", connectAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// attempt to disable checksums (let's pass through more errors!)
	// NOTE: This should only work on IPv4 sockets
	if conf.disableCsum {
		udpconn, ok := conn.(*net.UDPConn)
		if !ok {
			return fmt.Errorf("result of Dial is not net.UDPConn")
		}
		fd, err := udpconn.File()
		if err != nil {
			return err
		}
		err = sonocheck.SetNoCheck(fd)
		var successMessage string
		if err == nil {
			successMessage = "SUCCESS!"
		} else {
			successMessage = "failed (expected): " + err.Error()
		}
		fmt.Printf("disabling UDP checksums with setsockopt(SO_NO_CHECK): %s\n", successMessage)
	}

	rand.Seed(time.Now().UnixNano())
	timer := packetTimer{}

	buf := make([]byte, conf.packetLength)
	fillWithPattern(buf, conf)
	for {
		sendSlice := buf
		if conf.randomLength {
			sendSlice = buf[:rand.Intn(conf.packetLength)]
		}
		n, err := conn.Write(sendSlice)
		if err != nil {
			return err
		}
		if n != len(sendSlice) {
			return fmt.Errorf("wrote %d bytes; expected %d bytes", n, len(buf))
		}

		timer.increment(len(sendSlice))
		time.Sleep(sleepDuration)
	}
}

func runServer(listenPort int, conf *config) error {
	listenAddr := net.UDPAddr{net.IP([]byte{0, 0, 0, 0}), listenPort, ""}
	conn, err := net.ListenUDP("udp", &listenAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := make([]byte, conf.packetLength)
	expected := make([]byte, conf.packetLength)
	fillWithPattern(expected, conf)
	timer := packetTimer{}
	errors := 0
	for {
		// zero just in case
		for i := range buf {
			buf[i] = 0
		}

		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		timer.increment(n)
		// fmt.Printf("received %d from %s\n", n, clientAddr.String())
		inSlice := buf
		expectedSlice := expected
		if conf.randomLength {
			inSlice = buf[:n]
			expectedSlice = expected[:n]
		}
		if datautil.MatchWithErrors(inSlice, expectedSlice, conf.dumpOnError).HasErrors() {
			errors += 1
			fmt.Printf("Source: %s; %d errors out of %d packets\n",
				clientAddr.String(), errors, timer.totalPackets)
		}
	}
}

type mode int

const (
	modeSequence mode = iota
	modeFill
	modeRandom
)

type config struct {
	expectedMode  mode
	randomSeed    int
	expectedBytes []byte
	disableCsum   bool
	dumpOnError   bool
	randomLength  bool
	packetLength  int
	orMask        byte
}

func parseByte(s string) (byte, error) {
	if strings.HasPrefix(s, "0x") {
		s = s[2:len(s)]
	}
	byteVal, err := strconv.ParseUint(s, 16, 8)
	return byte(byteVal), err
}

func main() {
	listenPort := flag.Int("listen", 0, "start server listening on this port")
	connectAddr := flag.String("connect", "", "start clinet sending to this addr/port")
	dumpOnError := flag.Bool("dumpOnError", true, "dump packet contents on error")
	disableCsum := flag.Bool("disableCsum", false, "try to disable UDP checksums (SO_NO_CHECK)")
	randomLength := flag.Bool("randomLength", false, "randomize length of packets")
	length := flag.Int("length", maxPacketLength, "length of packet")
	rate := flag.Int("rate", 1000, "packets per second to send")
	rnd := flag.Int("rnd", 0, "send/expect pseudorandom with seed")
	fill := flag.String("fill", "", "hex string to write/expect in the packet (e.g. a9)")
	orMask := flag.String("orMask", "", "hex mask to OR with every expected byte in the packet")
	flag.Parse()

	if *length <= 0 || *length > maxPacketLength {
		fmt.Fprintf(os.Stderr, "ERROR: -length=%d; must be between 0 and %d\n",
			*length, maxPacketLength)
		os.Exit(1)
	}

	expectedMode := modeSequence
	var expectedBytes []byte = nil
	if *rnd != 0 {
		expectedMode = modeRandom
	}
	if *fill != "" {
		if expectedMode != modeSequence {
			fmt.Fprintln(os.Stderr, "Cannot specify more than one of -rnd, -fillByte")
			os.Exit(1)
		}
		var err error
		expectedBytes, err = hex.DecodeString(*fill)
		if err != nil {
			panic(err)
		}
		expectedMode = modeFill
	}
	orByte := byte(0)
	if *orMask != "" {
		var err error
		orByte, err = parseByte(*orMask)
		if err != nil {
			panic(err)
		}
	}
	conf := config{expectedMode, *rnd, expectedBytes, *disableCsum, *dumpOnError, *randomLength, *length, orByte}

	var err error
	if *listenPort != 0 {
		err = runServer(*listenPort, &conf)
	} else if len(*connectAddr) != 0 {
		err = runClient(*connectAddr, *rate, &conf)
	} else {
		fmt.Fprintln(os.Stderr, "Specify one of -connect or -listen")
		os.Exit(1)
		return
	}
	if err != nil {
		panic(err)
	}
}
