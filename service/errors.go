package service

type ServiceError struct {
	IsChunkUnwritten     bool
	IsFileNotFound       bool
	IsHashCollision      bool
	IsFilenameInvalid    bool
	IsRegexFailure       bool
	IsReplicationFailure bool
}

func (se *ServiceError) Error() string {
	if se.IsChunkUnwritten {
		return "The bytes range are not fully contained in the file buffer"
	}
	if se.IsFileNotFound {
		return "File not found"
	}
	if se.IsHashCollision {
		return "Hash collision identified"
	}
	if se.IsRegexFailure {
		return "Regex compilation failure"
	}
	if se.IsFilenameInvalid {
		return "Invalid file name"
	}
	if se.IsReplicationFailure {
		return "Could not obtain one or more chunks when replicating"
	}
	return "Undefined error"
}

func ReplicationFailedError() error {
	return &ServiceError{IsReplicationFailure: true}
}

func ChunkUnwrittenError() error {
	return &ServiceError{IsChunkUnwritten: true}
}

func FileNotFoundError() error {
	return &ServiceError{IsFileNotFound: true}
}

func HashCollisionEror() error {
	return &ServiceError{IsHashCollision: true}
}

func RegexFailureError() error {
	return &ServiceError{IsRegexFailure: true}
}

func InvalidFileNameError() error {
	return &ServiceError{IsFilenameInvalid: true}
}
