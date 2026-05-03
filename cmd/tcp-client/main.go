package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: tcp-client <host:port> <jwt-token>")
		os.Exit(1)
	}
	addr, token := os.Args[1], os.Args[2]

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("connected to", addr)

	auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
	conn.Write(append(auth, '\n'))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fmt.Println("←", scanner.Text())
	}
}
