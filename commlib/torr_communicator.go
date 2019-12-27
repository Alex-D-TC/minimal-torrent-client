package commlib

import (
	"fmt"
	"net"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/alex-d-tc/minimal-torrent-client/torr"
)

type TorrNode struct {
	Host  net.IP
	Port  uint16
	Name  string
	Index uint16
}

type TorrCommunicator struct {
	hubIP   net.IP
	hubPort uint16
}

type CommunicationAttempt struct {
	Host                 string
	Port                 uint32
	Name                 string
	Index                uint32
	IsCommunicationError bool
	IsMalformedResponse  bool
	IsInternalError      bool
}

type requestHandler func(*torr.Message, *CommunicationAttempt) bool
type LocalSearchResponseHandler func(*torr.LocalSearchResponse, *CommunicationAttempt)
type ChunkResponseHandler func(*torr.ChunkResponse, *CommunicationAttempt) bool

func MakeTorrCommunicator(hubIP net.IP, hubPort uint16) *TorrCommunicator {
	return &TorrCommunicator{
		hubIP:   hubIP,
		hubPort: hubPort,
	}
}

func (tc *TorrCommunicator) RegisterNode(node TorrNode) error {
	msg := &torr.Message{
		Type: torr.Message_REGISTRATION_REQUEST,
		RegistrationRequest: &torr.RegistrationRequest{
			Owner: node.Name,
			Index: int32(node.Index),
			Port:  int32(node.Port),
		},
	}

	rawMsg, err := proto.Marshal(msg)
	if err != nil {
		fmt.Println("RegisterNode error when marshaling registration request: ", err)
		return err
	}

	destAddr := &net.TCPAddr{
		IP:   tc.hubIP,
		Port: int(tc.hubPort),
	}

	attempt, resp := tc.handleHostCommunication(rawMsg, destAddr)
	if attempt.IsCommunicationError {
		return fmt.Errorf("Communication error")
	}
	if attempt.IsMalformedResponse {
		return fmt.Errorf("Malformed response")
	}
	if attempt.IsInternalError {
		return fmt.Errorf("Internal error")
	}

	regResponse := resp.GetRegistrationResponse()
	if regResponse.GetStatus() != torr.Status_SUCCESS {
		return fmt.Errorf(regResponse.GetErrorMessage())
	}

	return nil
}

func (tc *TorrCommunicator) GetNodesOfSubnet(subnetID int32) ([]TorrNode, error) {
	msg := &torr.Message{
		Type: torr.Message_SUBNET_REQUEST,
		SubnetRequest: &torr.SubnetRequest{
			SubnetId: subnetID,
		},
	}

	rawMsg, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("sendSubnetRequest proto.Marshal error: %s", err.Error())
	}

	destAddr := &net.TCPAddr{
		IP:   tc.hubIP,
		Port: int(tc.hubPort),
	}

	attempt, resp := tc.handleHostCommunication(rawMsg, destAddr)
	if attempt.IsCommunicationError {
		return nil, fmt.Errorf("Communication error")
	}
	if attempt.IsMalformedResponse {
		return nil, fmt.Errorf("Malformed response")
	}
	if attempt.IsInternalError {
		return nil, fmt.Errorf("Internal error")
	}

	subnetResp := resp.GetSubnetResponse()
	if subnetResp.GetStatus() != torr.Status_SUCCESS {
		return nil, fmt.Errorf("subnet request error: %s", subnetResp.GetErrorMessage())
	}

	return toTorrNode(subnetResp.GetNodes()), nil
}

func toTorrNode(torrNodes []*torr.NodeId) []TorrNode {
	nodes := []TorrNode{}
	for _, nodeID := range torrNodes {
		newNode := TorrNode{
			Host:  net.ParseIP(nodeID.GetHost()),
			Port:  uint16(nodeID.GetPort()),
			Name:  nodeID.GetOwner(),
			Index: uint16(nodeID.GetIndex()),
		}
		if nil == newNode.Host {
			fmt.Println("Received non-IP address for node: ", nodeID.GetHost(), " of name: ", nodeID.GetOwner(), " index: ", nodeID.GetIndex())
		} else {
			nodes = append(nodes, newNode)
		}
	}
	return nodes
}

func (tc *TorrCommunicator) SendLocalSearchRequest(nodes []TorrNode, regex string, handler LocalSearchResponseHandler) error {
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
	tc.roundRobinRequest(nodes, 0, rawMsg, responseHandler)
	return nil
}

func (tc *TorrCommunicator) SendChunkRequest(nodes []TorrNode, neighborStartIndex uint64, hash []byte, chunkIndex uint32, handler ChunkResponseHandler) error {
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
	return tc.roundRobinRequest(nodes, neighborStartIndex, rawMsg, responseHandler)
}

func (tc *TorrCommunicator) roundRobinRequest(nodes []TorrNode, neighborStartIndex uint64, rawMsg []byte, handler requestHandler) error {
	// Attempt sending the message to all known peers in the network
	nodeCount := uint64(len(nodes))
	for i := uint64(0); i < nodeCount; i++ {
		nodeIndex := (i + neighborStartIndex) % nodeCount
		node := nodes[nodeIndex]

		destAddr := &net.TCPAddr{
			IP:   node.Host,
			Port: int(node.Port),
		}
		attempt, response := tc.handleTorrCommunication(rawMsg, destAddr, node.Name, node.Index)
		shouldSearchContinue := handler(response, attempt)
		if !shouldSearchContinue {
			return nil
		}
	}
	return fmt.Errorf("Message was not handled by any known client")
}

func (tc *TorrCommunicator) handleHostCommunication(rawMsg []byte, destAddr *net.TCPAddr) (*CommunicationAttempt, *torr.Message) {
	return tc.handleTorrCommunication(rawMsg, destAddr, "host", 1)
}

func (tc *TorrCommunicator) handleTorrCommunication(rawMsg []byte, destAddr *net.TCPAddr, name string, index uint16) (*CommunicationAttempt, *torr.Message) {
	attempt := &CommunicationAttempt{
		Host:                 destAddr.IP.String(),
		Port:                 uint32(destAddr.Port),
		Name:                 name,
		Index:                uint32(index),
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
