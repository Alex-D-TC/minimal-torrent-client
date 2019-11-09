package internal

type ChunkUnwrittenError struct{}

// Implementation of the Error interface
func (cuw *ChunkUnwrittenError) Error() string {
	return "The bytes range are not fully contained in the file buffer"
}

type FileNotFoundError struct{}

// Implementation of the Error interface
func (fnf *FileNotFoundError) Error() string {
	return "File not found"
}

type HashCollisionError struct{}

// Implementation of the Error interface
func (hc *HashCollisionError) Error() string {
	return "Hash collision identified"
}
