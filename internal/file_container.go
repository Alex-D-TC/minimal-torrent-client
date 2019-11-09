package internal

import (
	"regexp"
)

type FileContainer struct {
	infoIndex      map[MD5Hash]*FileInfo
	fileSystemRoot string
}

func (fc *FileContainer) AddFile(hash MD5Hash, size int64, name string) error {
	fileInfo := makeFileInfo(hash, name, size)

	// Check for hash collisions or whether the file was already added
	foundFileInfo, exists := fc.infoIndex[hash]
	if exists && foundFileInfo.GetName() != name {
		return &HashCollisionError{}
	}
	if !exists {
		fc.infoIndex[hash] = fileInfo
	}

	return nil
}

func (fc *FileContainer) Search(regex string) ([]MD5Hash, error) {
	regexMatcher, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}

	var hashes []MD5Hash
	for _, fileItem := range fc.infoIndex {
		if regexMatcher.MatchString(fileItem.GetName()) {
			hashes = append(hashes, fileItem.GetHash())
		}
	}
	return hashes, nil
}

func (fc *FileContainer) GetFile(hash MD5Hash) (*FileInfo, error) {
	fi, exists := fc.infoIndex[hash]
	if !exists {
		return nil, &FileNotFoundError{}
	}
	return fi, nil
}
