package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"./datautil"
)

const packetPrint = 60000 // print after receiving this many packets

const tcpOverhead = 20 + 20 + 12 // IPv4 = 20 bytes; tcp = 20 bytes (usually) + 12 bytes timestamps (conservative)
const mtu = 1500                 // Ethernet standard
const maxPacketLength = mtu - tcpOverhead
const patternSeed = 0x1234567890abcdef

func assert(v bool) {
	if !v {
		panic("assertion failed")
	}
}

func runClient(connectAddr string, config *configuration) error {
	sleepDuration := time.Second / time.Duration(config.packetsPerSecond)
	conn, err := net.Dial("tcp", connectAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := make([]byte, maxPacketLength)
	echoed := make([]byte, maxPacketLength)
	filler := datautil.NewRandomFiller(patternSeed)
	for {
		filler.Fill(buf)
		n, err := conn.Write(buf)
		if err != nil {
			return err
		}
		assert(n == len(buf))

		if config.echo {
			n, err = conn.Read(echoed)
			if err != nil {
				return err
			}
			assert(n == len(buf))
			if datautil.MatchWithErrors(echoed, buf, config.dumpOnError).HasErrors() && config.panicOnError {
				panic("we found an error!?? panicing")
			}
		}

		time.Sleep(sleepDuration)
	}
}

func handleServerConnection(conn net.Conn, config *configuration) {
	err := serverConnection(conn, config)
	if err != nil {
		fmt.Printf("Error from client %s: %s\n", conn.RemoteAddr().String(), err.Error())
	}
}

func serverConnection(conn net.Conn, config *configuration) error {
	defer conn.Close()
	filler := datautil.NewRandomFiller(patternSeed)
	in := make([]byte, maxPacketLength)
	expected := make([]byte, maxPacketLength)

	foundShortPacket := false

	for {
		n, err := conn.Read(in)
		if err != nil {
			return err
		}
		if n < len(in) && !foundShortPacket {
			// we try to write single packets which should get delivered in one chunk
			fmt.Printf("unexpected short read: %d bytes < %d max bytes (disabling message)\n", n, len(in))
			foundShortPacket = true
		}

		inSlice := in[0:n]
		expectedSlice := expected[0:n]
		filler.Fill(expectedSlice)
		if datautil.MatchWithErrors(inSlice, expectedSlice, config.dumpOnError).HasErrors() {
			fmt.Printf("Error source: %s\n", conn.RemoteAddr().String())
		}

		if config.echo {
			n, err = conn.Write(inSlice)
			if err != nil {
				return err
			}
			assert(n == len(inSlice))
		}
	}
}

func runServer(listenPort int, config *configuration) error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(listenPort))
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go handleServerConnection(conn, config)
	}
}

type configuration struct {
	packetsPerSecond int
	echo             bool
	dumpOnError      bool
	panicOnError     bool
}

func main() {
	listenPort := flag.Int("listen", 0, "if not zero, listen on that port")
	connectAddr := flag.String("connect", "", "if not empty, send to this addr/port")
	echo := flag.Bool("echo", false, "server will echo client response back")
	rate := flag.Int("rate", 1000, "writes per second to send")
	panicOnError := flag.Bool("panicOnError", false, "panic when a client hits a mismatched packet")
	flag.Parse()

	config := configuration{*rate, *echo, true, *panicOnError}

	var err error
	if *listenPort != 0 {
		err = runServer(*listenPort, &config)
	} else if len(*connectAddr) != 0 {
		err = runClient(*connectAddr, &config)
	} else {
		fmt.Fprintln(os.Stderr, "Specify one of -listen or -connect")
		os.Exit(1)
		return
	}
	if err != nil {
		panic(err)
	}
}
