package internal

import (
	"fmt"

	"github.com/golang-collections/go-datastructures/augmentedtree"
)

// MD5Hash is a typedef to more easily work with md5 hashes
type MD5Hash [16]byte

// FileInfo is a struct which encapsulates the accessing of a file handled by the torrent node
type FileInfo struct {
	// a buffer representing the "file"
	data []byte

	// an augmented tree which holds all intervals where nothing has been written yet
	unwrittenIntervals augmentedtree.Tree

	hash MD5Hash
	name string
	size int64
}

// GetHash returns the MD5 hash of the file
func (fi *FileInfo) GetHash() MD5Hash {
	return fi.hash
}

// GetName returns the name of the file
func (fi *FileInfo) GetName() string {
	return fi.name
}

// GetSize returns the size of the file
func (fi *FileInfo) GetSize() int64 {
	return fi.size
}

// GetChunk reads a chunk from the managed file
// If parts of the chunk are not yet written in the file, the function returns an internal.ChunkUnwrittenError error
func (fi *FileInfo) GetChunk(offset int64, size int64) ([]byte, error) {
	// Check whether there are parts of the chunk which have not been written
	foundIntervals := fi.unwrittenIntervals.Query(&chunkInterval{left: offset, right: offset + size - 1})
	if len(foundIntervals) != 0 {
		return nil, &ChunkUnwrittenError{}
	}
	return fi.data[offset : offset+size], nil
}

// WriteChunk writes a chunk to the managed file
// If the data is to be written out of the bounds of the managed file, the function returns an internal.ChunkUnwrittenError error
func (fi *FileInfo) WriteChunk(offset int64, data []byte) error {
	dataLen := int64(len(data))
	fiDataLen := int64(len(fi.data))

	// Bounds checking
	if offset < 0 || offset > fiDataLen || offset+dataLen > fiDataLen {
		return &ChunkUnwrittenError{}
	}

	// Update the unwritten interval tree and write the data
	for i := offset; i < offset+dataLen; i++ {
		fi.data[i] = data[i]
	}
	markWrittenInterval(fi.unwrittenIntervals,
		&chunkInterval{left: offset, right: offset + int64(len(data)) - 1})
	return nil
}

func makeFileInfo(hash MD5Hash, name string, size int64) *FileInfo {

	unwrittenIntervals := augmentedtree.New(1)

	// Add the first unwritten interval
	unwrittenIntervals.Add(&chunkInterval{0, size - 1})

	return &FileInfo{
		data:               make([]byte, size),
		unwrittenIntervals: unwrittenIntervals,
		hash:               hash,
		name:               name,
		size:               size,
	}
}

// markWrittenInterval updates the unwritten interval tree when a new chunk interval is written
// any affected intervals are truncated or outright removed from the tree
func markWrittenInterval(rTree augmentedtree.Tree, writtenInterval *chunkInterval) {
	affectedIntervals := rTree.Query(writtenInterval)
	for _, interval := range affectedIntervals {
		// In either case, the interval is removed
		rTree.Delete(interval)

		// Try reinserting the left side of the interval, truncated
		if interval.LowAtDimension(1) < writtenInterval.left {
			rTree.Add(&chunkInterval{left: interval.LowAtDimension(1), right: writtenInterval.left - 1})
		}

		// Try reinserting the right side of the interval, truncated
		if interval.HighAtDimension(1) > writtenInterval.right {
			rTree.Add(&chunkInterval{left: writtenInterval.right + 1, right: interval.HighAtDimension(1)})
		}
	}
}

type chunkInterval struct {
	left  int64
	right int64
}

//
// Implementation of the augmentedtree.Interval interface
//

func (ci *chunkInterval) LowAtDimension(dimension uint64) int64 {
	// The chunk is designed to be used with a single dimension only
	if dimension != 1 {
		panic(fmt.Sprintf("chunkInterval requested is %d, maximum dimesion allowed is 1", dimension))
	}
	return ci.left
}

func (ci *chunkInterval) HighAtDimension(dimension uint64) int64 {
	// The chunk is designed to be used with a single dimension only
	if dimension != 1 {
		panic(fmt.Sprintf("chunkInterval requested is %d, maximum dimesion allowed is 1", dimension))
	}
	return ci.right
}

func (ci *chunkInterval) OverlapsAtDimension(target augmentedtree.Interval, dimension uint64) bool {
	// The chunk is designed to be used with a single dimension only
	if dimension != 1 {
		panic(fmt.Sprintf("chunkInterval requested is %d, maximum dimesion allowed is 1", dimension))
	}

	targetLow := target.LowAtDimension(dimension)
	targetHigh := target.HighAtDimension(dimension)

	return ci.left <= targetLow && ci.right >= targetLow && ci.right <= targetHigh ||
		ci.right >= targetHigh && ci.left <= targetHigh && ci.left >= targetLow
}

func (ci *chunkInterval) ID() uint64 {
	// Either left or right is sufficient as an ID, since the intervals should
	// 	not intersect by design
	return uint64(ci.left)
}
