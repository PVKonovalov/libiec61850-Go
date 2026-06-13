/*
 *  directory_browsing.go
 *
 *  Copyright 2014-2024 Michael Zillgith
 *  Copyright 2026 Pavel Konovalov Golang port
 *
 *  This file is part of libIEC61850.
 *
 *  libIEC61850 is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  libIEC61850 is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License
 *  along with libIEC61850.  If not, see <http://www.gnu.org/licenses/>.
 *
 *  See COPYING file for the complete license text.
 */

// directory_browsing prints the full IEC 61850 object hierarchy of a server.
//
// Output format (one tab per level):
//
//	LogicalDevice
//	  LogicalNode
//	    FunctionalConstraint
//	      DataObject
//	        DataAttribute
//	          ...
//
// Usage:
//
//	./directory_browsing [host] [port]
//
// Default host: localhost, Default port: 102
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/client"
)

// node is one element in the object tree.
type node struct {
	name     string
	children map[string]*node
}

func newNode(name string) *node {
	return &node{name: name, children: make(map[string]*node)}
}

// insert adds a $-separated path into the tree, creating intermediate nodes as needed.
func (n *node) insert(path string) {
	parts := strings.SplitN(path, "$", 2)
	child, ok := n.children[parts[0]]
	if !ok {
		child = newNode(parts[0])
		n.children[parts[0]] = child
	}
	if len(parts) == 2 {
		child.insert(parts[1])
	}
}

// print recursively prints the tree with tab indentation.
func (n *node) print(depth int) {
	fmt.Printf("%s%s\n", strings.Repeat("\t", depth), n.name)
	for _, child := range n.children {
		child.print(depth + 1)
	}
}

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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	address := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Connecting to %s\n", address)

	opts := client.DefaultOptions()
	opts.ConnectTimeout = 5 * time.Second
	opts.RequestTimeout = 30 * time.Second
	opts.IdleTimeout = 60 * time.Second
	conn, err := client.DialContext(ctx, address, opts)
	if err != nil {
		fmt.Printf("Failed to connect to %s: %v\n", address, err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected")

	devices, err := conn.GetServerDirectory()
	if err != nil {
		fmt.Printf("Failed to get server directory: %v\n", err)
		return
	}

	for _, device := range devices {
		fmt.Printf("%s\n", device)

		names, err := conn.GetLogicalDeviceDirectory(device)
		if err != nil {
			fmt.Printf("\t(error reading %s: %v)\n", device, err)
			continue
		}

		// Build a tree from the flat $-separated name list and print it.
		root := newNode(device)
		for _, name := range names {
			root.insert(name)
		}
		for _, child := range root.children {
			child.print(1)
		}
	}
}
