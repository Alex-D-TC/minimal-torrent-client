package service

import (
	"crypto/md5"
	"fmt"

	"github.com/alex-d-tc/minimal-torrent-client/commlib"
	"github.com/alex-d-tc/minimal-torrent-client/internal"
	"github.com/alex-d-tc/minimal-torrent-client/torr"
)

type TorrentService struct {
	torrCommunicator *commlib.TorrCommunicator
	fileContainer    *internal.FileContainer
	chunkSize        int64
}

type FileInfoView struct {
	// The fields in this struct are to be considered READ-ONLY
	Hash   internal.MD5Hash
	Size   int64
	Name   string
	Chunks []*ChunkInfoView
}

type ChunkInfoView struct {
	// The fields in this struct are to be considered READ-ONLY
	Index int64
	Size  int64
	Hash  internal.MD5Hash
}

type NodeReplicationAttempt struct {
	CommAttempt
	ChunkIndex uint32
}

type NodeSearchResult struct {
	CommAttempt
	FoundFiles []*torr.FileInfo
}

type CommAttempt struct {
	Host         string
	Port         uint32
	TorrStatus   torr.Status
	ErrorMessage string
}

func MakeTorrentService(chunkSize int64, ipPrefix [3]byte, portBase uint16, ipSuffixes []byte, portOffsets []uint16, hostIPSuffix byte, hostPortOffset uint16) *TorrentService {
	return &TorrentService{
		torrCommunicator: commlib.MakeTorrCommunicator(ipPrefix, portBase, ipSuffixes, portOffsets, hostIPSuffix, hostPortOffset),
		fileContainer:    internal.MakeFileContainer(),
		chunkSize:        chunkSize,
	}
}

func (service *TorrentService) Search(regex string) ([]*NodeSearchResult, error) {
	searchResults := []*NodeSearchResult{}
	responseHandler := func(resp *torr.LocalSearchResponse, attempt *commlib.CommunicationAttempt) {
		searchResult := &NodeSearchResult{
			CommAttempt: CommAttempt{
				Host:         attempt.Host,
				Port:         attempt.Port,
				TorrStatus:   torr.Status_UNABLE_TO_COMPLETE,
				ErrorMessage: "",
			},
			FoundFiles: []*torr.FileInfo{},
		}
		searchResults = append(searchResults, searchResult)
		if attempt.IsCommunicationError {
			searchResult.ErrorMessage = "Host unreachable"
			searchResult.TorrStatus = torr.Status_NETWORK_ERROR
			return
		}
		if attempt.IsMalformedResponse {
			searchResult.ErrorMessage = "Malformed message received"
			searchResult.TorrStatus = torr.Status_MESSAGE_ERROR
			return
		}
		searchResult.ErrorMessage = resp.GetErrorMessage()
		searchResult.TorrStatus = resp.GetStatus()
		searchResult.FoundFiles = resp.GetFileInfo()
	}
	err := service.torrCommunicator.SendLocalSearchRequest(regex, responseHandler)
	return searchResults, err
}

func (service *TorrentService) Replicate(hash internal.MD5Hash, name string, size int64) ([]*NodeReplicationAttempt, error) {
	// Input validation
	if len(name) == 0 {
		return nil, InvalidFileNameError()
	}

	// Check whether we have the file
	file, err := service.fileContainer.GetFile(hash)
	if err == nil && file != nil {
		return nil, nil
	}

	// Attempt to acquire the file
	return service.acquireFile(hash, name, size)
}

func (service *TorrentService) GetFileData(hash internal.MD5Hash) ([]byte, error) {
	file, err := service.fileContainer.GetFile(hash)
	if err != nil {
		return nil, service.handleInternalError(err)
	}
	return file.GetContents(), nil
}

func (service *TorrentService) GetChunk(hash internal.MD5Hash, chunkIndex int64) ([]byte, error) {
	file, err := service.fileContainer.GetFile(hash)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	chunkOffset := chunkIndex * service.chunkSize
	chunkSize := service.chunkSize
	if file.GetSize()-chunkOffset < service.chunkSize {
		chunkSize = file.GetSize() - chunkOffset
	}

	chunk, err := file.GetChunk(chunkOffset, chunkSize)
	if err != nil {
		return nil, service.handleInternalError(err)
	}
	return chunk, nil
}

func (service *TorrentService) Upload(filename string, data []byte) (*FileInfoView, error) {
	hash := md5.Sum(data)
	err := service.fileContainer.AddFile(hash, int64(len(data)), filename)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	file, err := service.fileContainer.GetFile(hash)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	err = file.WriteChunk(0, data)
	if err != nil {
		fmt.Println("[ERROR] TorrentService.Upload: ", err)
		return nil, err
	}

	return service.prepareFileInfoView(file)
}

func (service *TorrentService) LocalSearch(regex string) ([]*FileInfoView, error) {
	files, err := service.fileContainer.Search(regex)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	views, err := service.prepareFileInfoViews(files)
	if err != nil {
		return nil, err
	}
	return views, nil
}

// TODO: add a cache to have the chunk hashes be calculated only once
func (service *TorrentService) prepareFileInfoViews(files []*internal.FileInfo) ([]*FileInfoView, error) {

	var views []*FileInfoView
	for _, file := range files {
		view, err := service.prepareFileInfoView(file)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}

	return views, nil
}

func (service *TorrentService) prepareFileInfoView(file *internal.FileInfo) (*FileInfoView, error) {
	chunkViews, err := service.prepareChunkInfoViews(file)
	if err != nil {
		return nil, err
	}

	return &FileInfoView{
		Hash:   file.GetHash(),
		Name:   file.GetName(),
		Size:   file.GetSize(),
		Chunks: chunkViews,
	}, nil
}

func (service *TorrentService) prepareChunkInfoViews(file *internal.FileInfo) ([]*ChunkInfoView, error) {
	chunkCount := file.GetSize() / service.chunkSize
	if file.GetSize()%service.chunkSize != 0 {
		// Account for an extra, smaller chunk
		chunkCount++
	}

	var chunkViews []*ChunkInfoView
	for i := int64(0); i < chunkCount; i++ {
		chunkOffset := i * service.chunkSize
		chunkSize := service.chunkSize
		if chunkOffset+chunkSize > file.GetSize() {
			chunkSize = file.GetSize() - chunkOffset
		}
		chunk, err := file.GetChunk(chunkOffset, chunkSize)
		if err != nil {
			return nil, service.handleInternalError(err)
		}

		hash := md5.Sum(chunk)
		chunkView := &ChunkInfoView{
			Index: i,
			Size:  chunkSize,
			Hash:  hash,
		}
		chunkViews = append(chunkViews, chunkView)
	}

	return chunkViews, nil
}

func (service *TorrentService) acquireFile(hash internal.MD5Hash, name string, size int64) ([]*NodeReplicationAttempt, error) {
	// Construct the FileInfo
	err := service.fileContainer.AddFile(hash, size, name)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	file, err := service.fileContainer.GetFile(hash)
	if err != nil {
		return nil, service.handleInternalError(err)
	}

	// Attempt to request all the chunks
	// TODO: consider adding a RemoveFile function, to cleanup partially downloaded files
	return service.obtainChunks(file)
}

func (service *TorrentService) obtainChunks(file *internal.FileInfo) ([]*NodeReplicationAttempt, error) {
	chunkCount := file.GetSize() / service.chunkSize
	if file.GetSize()%service.chunkSize != 0 {
		chunkCount++
	}

	// Channel through which goroutines signal termination
	chunkRequestorChan := make(chan []*NodeReplicationAttempt, chunkCount)
	// Launch goroutines to fetch the chunks
	for i := int64(0); i < chunkCount; i++ {
		go service.fetchChunk(file, uint32(i), chunkRequestorChan)
	}

	replicationAttempts := []*NodeReplicationAttempt{}
	// Await all goroutines to finish
	everybodySucceeded := true
	for i := int64(0); i < chunkCount; i++ {
		chunkAttempts := <-chunkRequestorChan
		replicationAttempts = append(replicationAttempts, chunkAttempts...)
		everybodySucceeded = everybodySucceeded && anAttemptSucceeded(chunkAttempts)
	}

	var err error
	if !everybodySucceeded {
		err = ReplicationFailedError()
	}
	return replicationAttempts, err
}

func (service *TorrentService) fetchChunk(file *internal.FileInfo, chunkIndex uint32, requestorChan chan<- []*NodeReplicationAttempt) {
	attemptsForChunk := []*NodeReplicationAttempt{}
	responseHandler := func(resp *torr.ChunkResponse, attempt *commlib.CommunicationAttempt) bool {
		// Record chunk replication attempt
		replicationAttempt := &NodeReplicationAttempt{
			CommAttempt: CommAttempt{
				Host:         attempt.Host,
				Port:         attempt.Port,
				TorrStatus:   torr.Status_UNABLE_TO_COMPLETE,
				ErrorMessage: "",
			},
			ChunkIndex: chunkIndex,
		}
		if attempt.IsCommunicationError {
			replicationAttempt.TorrStatus = torr.Status_NETWORK_ERROR
			replicationAttempt.ErrorMessage = "Destination unreachable"
			attemptsForChunk = append(attemptsForChunk, replicationAttempt)
			return true
		}
		if attempt.IsMalformedResponse {
			replicationAttempt.TorrStatus = torr.Status_MESSAGE_ERROR
			replicationAttempt.ErrorMessage = "Malformed response received"
			attemptsForChunk = append(attemptsForChunk, replicationAttempt)
			return true
		}
		replicationAttempt.TorrStatus = resp.GetStatus()
		replicationAttempt.ErrorMessage = resp.GetErrorMessage()
		attemptsForChunk = append(attemptsForChunk, replicationAttempt)

		// Handle response
		if resp.GetStatus() != torr.Status_SUCCESS {
			fmt.Println("Chunk request failed with status ", resp.GetErrorMessage())
			return true
		}
		// Copy the chunk over
		data := resp.GetData()
		err := file.WriteChunk(int64(chunkIndex)*service.chunkSize, data)
		if err != nil {
			fmt.Println("Write chunk failed with error: ", err)
			return true
		}
		return false
	}
	hash := file.GetHash()
	err := service.torrCommunicator.SendChunkRequest(uint64(chunkIndex), hash[:], chunkIndex, responseHandler)
	if err != nil {
		// Log the error
		err = fmt.Errorf("Failed obtaining chunk: %d. Error: %s", chunkIndex, err.Error())
		fmt.Println(err.Error())
	}
	requestorChan <- attemptsForChunk
}

func anAttemptSucceeded(attempts []*NodeReplicationAttempt) bool {
	for _, attempt := range attempts {
		if attempt.TorrStatus == torr.Status_SUCCESS {
			return true
		}
	}
	return false
}

func (service *TorrentService) handleInternalError(err error) error {
	internalError := err.(*internal.InternalError)
	if internalError.IsChunkUnwritten {
		return ChunkUnwrittenError()
	}
	if internalError.IsFileNotFound {
		return FileNotFoundError()
	}
	if internalError.IsHashCollision {
		return HashCollisionEror()
	}
	if internalError.IsRegexFailure {
		return RegexFailureError()
	}
	if internalError.IsFilenameInvalid {
		return InvalidFileNameError()
	}
	return err
}
