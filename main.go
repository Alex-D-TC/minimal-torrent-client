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
	ip := net.ParseIP(os.Args[1])
	if ip == nil {
		panic("Could not parse ip")
	}

	port, err := strconv.ParseUint(os.Args[2], 10, 16)
	if err != nil {
		panic(err)
	}

	index, err := strconv.ParseUint(os.Args[3], 10, 16)
	if err != nil {
		panic(err)
	}

	hubIP := net.ParseIP("127.0.0.1")
	hubPort := uint16(51234)
	communicator := commlib.MakeTorrCommunicator(hubIP, hubPort)
	node := commlib.TorrNode{
		Host:  net.ParseIP("127.0.0.1"),
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
