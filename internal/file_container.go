package internal

import (
	"regexp"
)

type FileContainer struct {
	infoIndex map[MD5Hash]*FileInfo
}

func MakeFileContainer() *FileContainer {
	return &FileContainer{
		infoIndex: map[MD5Hash]*FileInfo{},
	}
}

func (fc *FileContainer) AddFile(hash MD5Hash, size int64, name string) error {
	if len(name) == 0 {
		return InvalidFileNameError()
	}

	// Check for hash collisions or whether the file was already added
	foundFileInfo, exists := fc.infoIndex[hash]
	if exists && foundFileInfo.GetName() != name {
		return HashCollisionEror()
	}
	if !exists {
		fc.infoIndex[hash] = makeFileInfo(hash, name, size)
	}

	return nil
}

func (fc *FileContainer) Search(regex string) ([]*FileInfo, error) {
	regexMatcher, err := regexp.Compile(regex)
	if err != nil {
		return nil, RegexFailureError()
	}

	var files []*FileInfo
	for _, fileItem := range fc.infoIndex {
		if regexMatcher.MatchString(fileItem.GetName()) {
			files = append(files, fileItem)
		}
	}
	return files, nil
}

func (fc *FileContainer) GetFile(hash MD5Hash) (*FileInfo, error) {
	fi, exists := fc.infoIndex[hash]
	if !exists {
		return nil, FileNotFoundError()
	}
	return fi, nil
}
