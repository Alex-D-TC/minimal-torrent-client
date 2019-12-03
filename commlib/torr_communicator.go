package commlib

import (
	"fmt"
	"net"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/alex-d-tc/minimal-torrent-client/torr"
)

type TorrCommunicator struct {
	ipPrefix    [3]byte
	portBase    uint16
	ipSuffixes  []byte
	portOffsets []uint16

	hostIPSuffix   byte
	hostPortOffset uint16
}

type CommunicationAttempt struct {
	Host                 string
	Port                 uint32
	IsCommunicationError bool
	IsMalformedResponse  bool
	IsInternalError      bool
}

type requestHandler func(*torr.Message, *CommunicationAttempt) bool
type LocalSearchResponseHandler func(*torr.LocalSearchResponse, *CommunicationAttempt)
type ChunkResponseHandler func(*torr.ChunkResponse, *CommunicationAttempt) bool

func MakeTorrCommunicator(ipPrefix [3]byte, portBase uint16, ipSuffixes []byte, portOffsets []uint16, hostIPSuffix byte, hostPortOffset uint16) *TorrCommunicator {
	return &TorrCommunicator{
		ipPrefix:       ipPrefix,
		portBase:       portBase,
		ipSuffixes:     ipSuffixes,
		portOffsets:    portOffsets,
		hostIPSuffix:   hostIPSuffix,
		hostPortOffset: hostPortOffset,
	}
}

func (tc *TorrCommunicator) SendLocalSearchRequest(regex string, handler LocalSearchResponseHandler) error {
	// Assemble the local search request
	msg := &torr.Message{
		Type: torr.Message_LOCAL_SEARCH_REQUEST,
		LocalSearchRequest: &torr.LocalSearchRequest{
			Regex: regex,
		},
	}
	responseHandler := func(msg *torr.Message, attempt *CommunicationAttempt) bool {
		if attempt.IsCommunicationError || attempt.IsMalformedResponse || attempt.IsInternalError {
			handler(nil, attempt)
			return true
		}
		if msg.GetType() == torr.Message_LOCAL_SEARCH_RESPONSE {
			handler(msg.GetLocalSearchResponse(), attempt)
			return true
		}
		// Log invalid message type received
		fmt.Println("Received message of type: ", msg.GetType(), "expected: ", torr.Message_LOCAL_SEARCH_REQUEST)
		attempt.IsMalformedResponse = true
		handler(nil, attempt)
		return true
	}
	rawMsg, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	tc.roundRobinRequest(false, 0, rawMsg, responseHandler)
	return nil
}

func (tc *TorrCommunicator) SendChunkRequest(neighborStartIndex uint64, hash []byte, chunkIndex uint32, handler ChunkResponseHandler) error {
	// Assemble the chunk request
	msg := &torr.Message{
		Type: torr.Message_CHUNK_REQUEST,
		ChunkRequest: &torr.ChunkRequest{
			FileHash:   hash,
			ChunkIndex: chunkIndex,
		},
	}
	responseHandler := func(msg *torr.Message, attempt *CommunicationAttempt) bool {
		if attempt.IsCommunicationError || attempt.IsMalformedResponse || attempt.IsInternalError {
			return handler(nil, attempt)
		}
		if msg.GetType() == torr.Message_CHUNK_RESPONSE {
			return handler(msg.GetChunkResponse(), attempt)
		}
		// Log invalid message type received
		fmt.Println("Received message of type: ", msg.GetType(), "expected: ", torr.Message_LOCAL_SEARCH_REQUEST)
		attempt.IsMalformedResponse = true
		return handler(nil, attempt)
	}
	rawMsg, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return tc.roundRobinRequest(true, neighborStartIndex, rawMsg, responseHandler)
}

func (tc *TorrCommunicator) roundRobinRequest(skipHost bool, neighborStartIndex uint64, rawMsg []byte, handler requestHandler) error {
	// Attempt sending the message to all known peers in the network
	for ipIdx := range tc.ipSuffixes {
		for portIdx := range tc.portOffsets {

			realIPIdx := (uint64(ipIdx) + neighborStartIndex) % uint64(len(tc.ipSuffixes))
			realPortIDX := (uint64(portIdx) + neighborStartIndex) % uint64(len(tc.portOffsets))

			if skipHost && tc.ipSuffixes[realIPIdx] == tc.hostIPSuffix && tc.portOffsets[realPortIDX] == tc.hostPortOffset {
				// Skip contacting myself
				continue
			}

			destAddr := &net.TCPAddr{
				IP:   net.IPv4(tc.ipPrefix[0], tc.ipPrefix[1], tc.ipPrefix[2], tc.ipSuffixes[realIPIdx]),
				Port: int(tc.portBase + tc.portOffsets[realPortIDX]),
			}
			attempt, response := tc.handleCommunication(rawMsg, destAddr)
			shouldSearchContinue := handler(response, attempt)
			if !shouldSearchContinue {
				return nil
			}
		}
	}

	return fmt.Errorf("Message was not handled by any known client")
}

func (tc *TorrCommunicator) handleCommunication(rawMsg []byte, destAddr *net.TCPAddr) (*CommunicationAttempt, *torr.Message) {
	attempt := &CommunicationAttempt{
		Host:                 destAddr.IP.String(),
		Port:                 uint32(destAddr.Port),
		IsCommunicationError: false,
		IsMalformedResponse:  false,
		IsInternalError:      false,
	}

	connection, err := net.DialTCP("tcp4", nil, destAddr)
	if err != nil {
		// Log error
		fmt.Println("DialTCP Error occurred: ", err)
		attempt.IsCommunicationError = true
		return attempt, nil
	}
	defer connection.Close()

	err = WriteMessage(connection, rawMsg)
	if err != nil {
		// Log error
		fmt.Println("WriteMessage Error occurred: ", err)
		attempt.IsCommunicationError = true
		return attempt, nil
	}

	// Await response. 2 second deadline
	err = connection.SetDeadline(time.Now().Add(time.Second * 2))
	if err != nil {
		// Log error
		fmt.Println("[ERROR] TorrentCommunicator.roundRobinRequest: Could not set deadline to connection: ", err)
		attempt.IsInternalError = true
		return attempt, nil
	}

	rawResponse, err := ReadMessage(connection)
	if err != nil {
		// Log error
		fmt.Println("ReadMessage Error occurred: ", err)
		attempt.IsCommunicationError = true
		return attempt, nil
	}

	var response torr.Message
	err = proto.Unmarshal(rawResponse, &response)
	if err != nil {
		// Log error
		fmt.Println("proto.Unmarshal Error occurred: ", err)
		attempt.IsMalformedResponse = true
		return attempt, nil
	}

	return attempt, &response
}
