package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", os.Args[1])
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for i := 0; true; i++ {
		message := fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa %d\n", i)
		_, err = conn.Write([]byte(message))
		if err != nil {
			panic(err)
		}

		time.Sleep(time.Second)
	}
}
