package internal

type InternalError struct {
	IsChunkUnwritten  bool
	IsFileNotFound    bool
	IsHashCollision   bool
	IsRegexFailure    bool
	IsFilenameInvalid bool
}

func (ie *InternalError) Error() string {
	if ie.IsChunkUnwritten {
		return "The bytes range are not fully contained in the file buffer"
	}
	if ie.IsFileNotFound {
		return "File not found"
	}
	if ie.IsHashCollision {
		return "Hash collision identified"
	}
	if ie.IsRegexFailure {
		return "Regex compilation failure"
	}
	if ie.IsFilenameInvalid {
		return "Invalid file name"
	}
	return "Undefined error"
}

func ChunkUnwrittenError() error {
	return &InternalError{IsChunkUnwritten: true}
}

func FileNotFoundError() error {
	return &InternalError{IsFileNotFound: true}
}

func HashCollisionEror() error {
	return &InternalError{IsHashCollision: true}
}

func RegexFailureError() error {
	return &InternalError{IsRegexFailure: true}
}

func InvalidFileNameError() error {
	return &InternalError{IsFilenameInvalid: true}
}
