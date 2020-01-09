package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/alex-d-tc/minimal-torrent-client/commlib"
	"github.com/alex-d-tc/minimal-torrent-client/communication"
	"github.com/alex-d-tc/minimal-torrent-client/service"
)

func main() {
	hubIP := net.ParseIP(os.Args[1])
	if hubIP == nil {
		panic("Could not parse ip")
	}

	hubPort, err := strconv.ParseUint(os.Args[2], 10, 16)
	if err != nil {
		panic(err)
	}

	nodeIP := net.ParseIP(os.Args[3])
	if nodeIP == nil {
		panic("Could not parse node IP")
	}

	port, err := strconv.ParseUint(os.Args[4], 10, 16)
	if err != nil {
		panic(err)
	}

	index, err := strconv.ParseUint(os.Args[5], 10, 16)
	if err != nil {
		panic(err)
	}

	fmt.Println("Connecting to host", hubIP, "on port", hubPort)

	communicator := commlib.MakeTorrCommunicator(hubIP, uint16(hubPort))
	node := commlib.TorrNode{
		Host:  nodeIP,
		Port:  uint16(port),
		Name:  "Alex",
		Index: uint16(index),
	}
	err = communicator.RegisterNode(node)
	if err != nil {
		panic(fmt.Errorf("Could not register node: %s", err.Error()))
	}

	service := service.MakeTorrentService(1024, communicator)
	localListener := communication.MakeLocalListener(uint16(port), service)
	localListener.Listen()
}
