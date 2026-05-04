package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	mangaFlag := flag.String("manga-ids", "", "comma-separated manga IDs to subscribe to (empty = all)")
	flag.Parse()

	serverAddr := os.Getenv("UDP_SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:9091"
	}

	srv, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Fatalf("udp-client: resolve %s: %v", serverAddr, err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		log.Fatalf("udp-client: listen: %v", err)
	}
	defer conn.Close()

	var mangaIDs []string
	if *mangaFlag != "" {
		mangaIDs = strings.Split(*mangaFlag, ",")
	}

	payload, _ := json.Marshal(map[string]any{"type": "register", "manga_ids": mangaIDs})
	if _, err := conn.WriteToUDP(payload, srv); err != nil {
		log.Fatalf("udp-client: register: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		signal.Stop(quit)
		unreg, _ := json.Marshal(map[string]string{"type": "unregister"})
		conn.WriteToUDP(unreg, srv) //nolint:errcheck
		conn.Close()
	}()

	fmt.Printf("subscribed to %v — waiting for notifications (Ctrl+C to quit)...\n", mangaIDs)

	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // closed by signal handler
		}
		var msg map[string]any
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "notification":
			fmt.Printf("[notification] %s — %s\n", msg["manga_id"], msg["message"])
		case "ack":
			fmt.Printf("[ack] %s\n", msg["message"])
		}
	}
}
