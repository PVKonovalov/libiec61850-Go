package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/client"
)

func main() {
	host := "localhost"
	port := 102

	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if len(os.Args) > 2 {
		var err error
		port, err = strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port: %v\n", err)
			os.Exit(1)
		}
	}

	address := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Connecting to %s\n", address)

	conn, err := client.Dial(address)
	if err != nil {
		fmt.Printf("Failed to connect to %s: %v\n", address, err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected")

	// List logical devices on the server
	devices, err := conn.GetServerDirectory()
	if err != nil {
		fmt.Printf("Failed to get server directory: %v\n", err)
	}

	for _, device := range devices {
		fmt.Printf("%s\n", device)
		nodes, err := conn.GetLogicalDeviceDirectory(device)
		if err != nil {
			fmt.Printf("Failed to get logical device directory: %v\n", err)
		} else {
			for _, node := range nodes {
				fmt.Printf("%s\n", node)
			}
		}
	}
}
