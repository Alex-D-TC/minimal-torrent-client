package communication

import (
	"fmt"
	"net"

	"github.com/golang/protobuf/proto"

	"github.com/alex-d-tc/minimal-torrent-client/commlib"
	"github.com/alex-d-tc/minimal-torrent-client/internal"
	"github.com/alex-d-tc/minimal-torrent-client/service"
	"github.com/alex-d-tc/minimal-torrent-client/torr"
)

type LocalListener struct {
	port    uint16
	service *service.TorrentService
}

func MakeLocalListener(port uint16, service *service.TorrentService) *LocalListener {
	return &LocalListener{
		port:    port,
		service: service,
	}
}

func (listener *LocalListener) Listen() {
	listeningString := fmt.Sprintf("localhost:%d", listener.port)
	fmt.Println("Listening on: ", listeningString)
	socket, err := net.Listen("tcp", listeningString)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := socket.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}

		go listener.handleConn(conn)
	}
}

func (listener *LocalListener) handleConn(conn net.Conn) {
	buffer, err := commlib.ReadMessage(conn)
	if err != nil {
		fmt.Println(err)
		return
	}

	var torrentMsg torr.Message
	err = proto.Unmarshal(buffer, &torrentMsg)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = listener.handleTorrentMessage(conn, &torrentMsg)
	if err != nil {
		fmt.Println("[ERROR] handleConn: ", err)
	}

	conn.Close()
}

func (listener *LocalListener) handleTorrentMessage(conn net.Conn, msg *torr.Message) error {
	switch msg.GetType() {
	case torr.Message_CHUNK_REQUEST:
		fmt.Println("Received chunk request ", msg.GetChunkRequest().GetChunkIndex(), msg.GetChunkRequest().GetFileHash())
		return listener.handleChunkRequest(conn, msg.GetChunkRequest())
	case torr.Message_LOCAL_SEARCH_REQUEST:
		fmt.Println("Received local search request")
		return listener.handleLocalSearchRequest(conn, msg.GetLocalSearchRequest())
	case torr.Message_SEARCH_REQUEST:
		fmt.Println("Received search request")
		return listener.handleSearchRequest(conn, msg.GetSearchRequest())
	case torr.Message_REPLICATE_REQUEST:
		fmt.Println("Received replicate request ", msg.GetReplicateRequest().GetFileInfo().GetHash())
		return listener.handleReplicateRequest(conn, msg.GetReplicateRequest())
	case torr.Message_DOWNLOAD_REQUEST:
		fmt.Println("Received download request")
		return listener.handleDownloadRequest(conn, msg.GetDownloadRequest())
	case torr.Message_UPLOAD_REQUEST:
		fmt.Println("Received upload request")
		return listener.handleUploadRequest(conn, msg.GetUploadRequest())
	default:
		return fmt.Errorf("Message from %s had an invalid type code: %s", conn.RemoteAddr().String(), msg.GetType().String())
	}
}

func (listener *LocalListener) handleChunkRequest(conn net.Conn, msg *torr.ChunkRequest) error {
	var err error
	var chunk []byte
	var errorMessage string
	respStatus := torr.Status_PROCESSING_ERROR

	hashSlice := msg.GetFileHash()
	if len(hashSlice) != len(internal.MD5Hash{}) {
		errorMessage = fmt.Sprintf("file hash length is not of %d bytes", len(internal.MD5Hash{}))
		respStatus = torr.Status_MESSAGE_ERROR
		fmt.Println("[ERROR] ChunkRequestHandler: ", errorMessage)
		return sendResponse(conn, prepareChunkResponse(respStatus, errorMessage, chunk))
	}

	hash := internal.MD5Hash{}
	for i := 0; i < len(hash); i++ {
		hash[i] = hashSlice[i]
	}
	chunk, err = listener.service.GetChunk(hash, int64(msg.GetChunkIndex()))
	if err != nil {
		serviceError := err.(*service.ServiceError)
		if serviceError.IsChunkUnwritten || serviceError.IsFileNotFound {
			respStatus = torr.Status_UNABLE_TO_COMPLETE
		} else {
			respStatus = torr.Status_PROCESSING_ERROR
		}
		errorMessage = serviceError.Error()
	} else {
		respStatus = torr.Status_SUCCESS
	}

	respStatus = torr.Status_SUCCESS
	return sendResponse(conn, prepareChunkResponse(respStatus, errorMessage, chunk))
}

func (listener *LocalListener) handleLocalSearchRequest(conn net.Conn, msg *torr.LocalSearchRequest) error {
	fileInfoViews, err := listener.service.LocalSearch(msg.GetRegex())
	if err != nil {
		fmt.Println(err)

		// Return an appropriate error response
		respStatus := torr.Status_PROCESSING_ERROR
		serviceError := err.(*service.ServiceError)
		if serviceError.IsRegexFailure {
			respStatus = torr.Status_MESSAGE_ERROR
		}
		resp := prepareLocalSearchResponse(
			respStatus, serviceError.Error(), nil)
		buff, err := proto.Marshal(resp)
		if err != nil {
			return err
		}

		return commlib.WriteMessage(conn, buff)
	}

	// Write the response buffer
	resp := prepareLocalSearchResponse(
		torr.Status_SUCCESS, "", prepareFileInfos(fileInfoViews))
	buff, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	return commlib.WriteMessage(conn, buff)
}

func (listener *LocalListener) handleUploadRequest(conn net.Conn, msg *torr.UploadRequest) error {
	var response *torr.Message
	infoView, err := listener.service.Upload(msg.GetFilename(), msg.GetData())
	if err != nil {
		status := torr.Status_PROCESSING_ERROR
		serviceError := err.(*service.ServiceError)
		if serviceError.IsFilenameInvalid {
			status = torr.Status_MESSAGE_ERROR
		}
		response = prepareUploadResponse(
			status, serviceError.Error(), nil)
	} else {
		response = prepareUploadResponse(
			torr.Status_SUCCESS, "", prepareFileInfo(infoView))
	}

	respBuffer, err := proto.Marshal(response)
	if err != nil {
		fmt.Println("[ERROR] handleUploadRequest: ", err)
		return err
	}
	return commlib.WriteMessage(conn, respBuffer)
}

func (listener *LocalListener) handleReplicateRequest(conn net.Conn, msg *torr.ReplicateRequest) error {
	var response *torr.Message
	hash := internal.MD5Hash{}
	if len(msg.GetFileInfo().GetHash()) != len(hash) {
		response = prepareReplicateResponse(torr.Status_PROCESSING_ERROR, "Provided hash is not of the size of an MD5 hash", nil)
		responseBuffer, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return commlib.WriteMessage(conn, responseBuffer)
	}
	for i, elem := range msg.GetFileInfo().GetHash() {
		hash[i] = elem
	}

	results, err := listener.service.Replicate(
		hash,
		msg.GetFileInfo().GetFilename(),
		int64(msg.GetFileInfo().GetSize()))
	if err != nil {
		serviceError := err.(*service.ServiceError)
		if serviceError.IsFilenameInvalid {
			response = prepareReplicateResponse(torr.Status_MESSAGE_ERROR, serviceError.Error(), nil)
		} else if serviceError.IsReplicationFailure {
			response = prepareReplicateResponse(torr.Status_UNABLE_TO_COMPLETE, serviceError.Error(), nil)
		} else {
			response = prepareReplicateResponse(torr.Status_PROCESSING_ERROR, serviceError.Error(), nil)
		}
	} else {
		response = prepareReplicateResponse(torr.Status_SUCCESS, "", results)
	}

	responseBuffer, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return commlib.WriteMessage(conn, responseBuffer)
}

func (listener *LocalListener) handleDownloadRequest(conn net.Conn, msg *torr.DownloadRequest) error {
	// Preinit response buffer fields
	status := torr.Status_PROCESSING_ERROR
	errorMessage := ""
	var data []byte

	// Parse the input hash
	hashSlice := msg.GetFileHash()
	if len(hashSlice) != len(internal.MD5Hash{}) {
		status = torr.Status_MESSAGE_ERROR
		errorMessage = "Hash length is not 16 bytes"
	} else {
		// Copy the hash to an MD5 hash
		var hash internal.MD5Hash
		for i := 0; i < len(hash); i++ {
			hash[i] = hashSlice[i]
		}

		// Get the contents
		var err error
		data, err = listener.service.GetFileData(hash)
		if err != nil {
			serviceErr := err.(*service.ServiceError)
			if serviceErr.IsFileNotFound {
				status = torr.Status_UNABLE_TO_COMPLETE
			}
			errorMessage = serviceErr.Error()
			data = nil
		} else {
			status = torr.Status_SUCCESS
		}
	}

	response := prepareDownloadResponse(
		status, errorMessage, data)
	responseBuffer, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return commlib.WriteMessage(conn, responseBuffer)
}

func (listener *LocalListener) handleSearchRequest(conn net.Conn, msg *torr.SearchRequest) error {
	var response *torr.Message
	results, err := listener.service.Search(msg.GetRegex())
	if err != nil {
		serviceError := err.(*service.ServiceError)
		if serviceError.IsRegexFailure {
			response = prepareSearchResponse(torr.Status_MESSAGE_ERROR, serviceError.Error(), nil)
		} else {
			response = prepareSearchResponse(torr.Status_PROCESSING_ERROR, serviceError.Error(), nil)
		}
	} else {
		response = prepareSearchResponse(torr.Status_SUCCESS, "", results)
	}

	responseBuffer, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return commlib.WriteMessage(conn, responseBuffer)
}

func prepareReplicateResponse(status torr.Status, errorMessage string, result []*service.NodeReplicationAttempt) *torr.Message {
	return &torr.Message{
		Type: torr.Message_REPLICATE_RESPONSE,
		ReplicateResponse: &torr.ReplicateResponse{
			Status:         status,
			ErrorMessage:   errorMessage,
			NodeStatusList: prepareNodeReplicationStatuses(result),
		},
	}
}

func prepareSearchResponse(status torr.Status, errorMessage string, result []*service.NodeSearchResult) *torr.Message {
	return &torr.Message{
		Type: torr.Message_SEARCH_RESPONSE,
		SearchResponse: &torr.SearchResponse{
			Status:       status,
			ErrorMessage: errorMessage,
			Results:      prepareNodeSearchResults(result),
		},
	}
}

func prepareNodeSearchResults(results []*service.NodeSearchResult) []*torr.NodeSearchResult {
	torrResults := []*torr.NodeSearchResult{}
	for _, res := range results {
		torrResults = append(torrResults, prepareNodeSearchResult(res))
	}
	return torrResults
}

func prepareNodeSearchResult(result *service.NodeSearchResult) *torr.NodeSearchResult {
	return &torr.NodeSearchResult{
		Node:         prepareNodeMessage(result.Host, result.Port),
		Status:       result.TorrStatus,
		ErrorMessage: result.ErrorMessage,
		Files:        result.FoundFiles,
	}
}

func prepareNodeMessage(host string, port uint32) *torr.Node {
	return &torr.Node{
		Host: host,
		Port: int32(port),
	}
}

func prepareNodeReplicationStatuses(results []*service.NodeReplicationAttempt) []*torr.NodeReplicationStatus {
	torrStatuses := []*torr.NodeReplicationStatus{}
	for _, status := range results {
		torrStatuses = append(torrStatuses, prepareNodeReplicationStatus(status))
	}
	return torrStatuses
}

func prepareNodeReplicationStatus(status *service.NodeReplicationAttempt) *torr.NodeReplicationStatus {
	return &torr.NodeReplicationStatus{
		Node:         prepareNodeMessage(status.Host, status.Port),
		ChunkIndex:   status.ChunkIndex,
		Status:       status.TorrStatus,
		ErrorMessage: status.ErrorMessage,
	}
}

func prepareFileInfos(infoViews []*service.FileInfoView) []*torr.FileInfo {
	var fileInfos []*torr.FileInfo
	for _, fileInfoView := range infoViews {
		fileInfos = append(fileInfos, prepareFileInfo(fileInfoView))
	}
	return fileInfos
}

func prepareFileInfo(infoView *service.FileInfoView) *torr.FileInfo {
	return &torr.FileInfo{
		Hash:     infoView.Hash[:],
		Size:     uint32(infoView.Size),
		Filename: infoView.Name,
		Chunks:   prepareChunks(infoView.Chunks),
	}
}

func prepareChunks(chunkViews []*service.ChunkInfoView) []*torr.ChunkInfo {
	var chunks []*torr.ChunkInfo
	for _, chunkView := range chunkViews {
		chunks = append(chunks, prepareChunk(chunkView))
	}
	return chunks
}

func prepareChunk(chunkView *service.ChunkInfoView) *torr.ChunkInfo {
	return &torr.ChunkInfo{
		Index: uint32(chunkView.Index),
		Size:  uint32(chunkView.Size),
		Hash:  chunkView.Hash[:],
	}
}

func prepareChunkResponse(status torr.Status, errorMessage string, chunkData []byte) *torr.Message {
	return &torr.Message{
		Type: torr.Message_CHUNK_RESPONSE,
		ChunkResponse: &torr.ChunkResponse{
			Status:       status,
			ErrorMessage: errorMessage,
			Data:         chunkData,
		},
	}
}

func prepareDownloadResponse(status torr.Status, errorMessage string, data []byte) *torr.Message {
	return &torr.Message{
		Type: torr.Message_DOWNLOAD_RESPONSE,
		DownloadResponse: &torr.DownloadResponse{
			Status:       status,
			ErrorMessage: errorMessage,
			Data:         data,
		},
	}
}

func prepareUploadResponse(status torr.Status, errorMessage string, fileInfo *torr.FileInfo) *torr.Message {
	return &torr.Message{
		Type: torr.Message_UPLOAD_RESPONSE,
		UploadResponse: &torr.UploadResponse{
			Status:       status,
			ErrorMessage: errorMessage,
			FileInfo:     fileInfo,
		},
	}
}

func prepareLocalSearchResponse(status torr.Status, errorMessage string, fileInfos []*torr.FileInfo) *torr.Message {
	return &torr.Message{
		Type: torr.Message_LOCAL_SEARCH_RESPONSE,
		LocalSearchResponse: &torr.LocalSearchResponse{
			Status:       status,
			ErrorMessage: errorMessage,
			FileInfo:     fileInfos,
		},
	}
}

func sendResponse(conn net.Conn, message *torr.Message) error {
	buff, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	return commlib.WriteMessage(conn, buff)
}
