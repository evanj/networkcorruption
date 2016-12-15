# Testing for network corruption

It turns out that sometimes network hardware goes bad and can corrupt a lot of data! I wrote some quick and dirty tools to try to help track it down.


## tcpcorrupt.go: TCP client/server to detect corrupt data

The client initializes a random number generator with a known fixed seed. It then writes one packet of data and sleeps (default: 1 ms). The server has the same seed, so it verifies that each byte matches the expected value on the connection.

If you pass the `-echo` flag to both the server and the client, the server will echo the data it receives from the client, and the client will verify it


## udpdata.go: UDP test client/server

The client sends UDP packets with different patterns, while the server verifies the contents and prints error messages. It attempts to use `setsockopt(SO_NO_CHECK)` to disable UDP checksums (optional in [UDP on IPv4](https://en.wikipedia.org/wiki/User_Datagram_Protocol#Packet_structure)).


### Quick start

1. Start a server listening on UDP port 12345: `./udpdata -listen=12345`
2. Start a client sending to it: `./udpdata -disableCsum -connect=localhost:12345`

Run `./udpdata -help` for all command line flags. If the server finds an error, it will print details to stdout.


### How it works

The client fills a byte slice with a _pattern_, and sends it to the server, at a given _rate_ (a bit under 1000 requests/second). The server checks that each byte matches the expected pattern. If it doesn't, it prints the bytes that did not match, and the hex dump of the corrupted message. As a result, you must pass the same "pattern flags" to the client and the server.


#### Available Patterns:
* _default_: Sequential: 0x00, 0x01, ..., 0xff, 0x00 etc.
* `-rnd`: Pseudo-random: seeds a random number generator with a fixed seed. Uses it to fill the packet.
* `-fillByte=(hex value)`: Static: fills the packet with a the given byte (in hex).
