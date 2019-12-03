package main

import (
	"github.com/alex-d-tc/minimal-torrent-client/communication"
	"github.com/alex-d-tc/minimal-torrent-client/service"
)

func main() {
	service := service.MakeTorrentService(1024, [3]byte{127, 0, 0}, 10000, []byte{1}, []uint16{1, 2, 3}, 1, 1)
	localListener := communication.MakeLocalListener(10000+1, service)
	localListener.Listen()
}
