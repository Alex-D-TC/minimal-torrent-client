package commlib

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

func WriteMessage(conn net.Conn, buff []byte) error {
	err := writeMessageSize(conn, uint32(len(buff)))
	if err != nil {
		fmt.Println("[ERROR] WriteMessage: ", err)
		return err
	}
	return writeAll(conn, buff)
}

func ReadMessage(conn net.Conn) ([]byte, error) {
	buffSize, err := readMessageSize(conn)
	if err != nil {
		return nil, err
	}

	finalBuff := make([]byte, buffSize)
	err = readAll(conn, finalBuff)
	return finalBuff, err
}

func writeAll(conn net.Conn, buffer []byte) error {
	toWrite := len(buffer)
	for toWrite > 0 {
		written, err := conn.Write(buffer)
		if err != nil {
			return err
		}
		toWrite -= written
	}
	return nil
}

func readAll(conn net.Conn, buffer []byte) error {
	gotEOF := false
	leftToRead := len(buffer)
	for !gotEOF && leftToRead > 0 {
		readBytes, err := conn.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Println("commlib.readAll: Encountered unexpected error ", err)
			return err
		}
		if err == io.EOF {
			gotEOF = true
		}
		leftToRead -= readBytes
	}
	return nil
}

func writeMessageSize(conn net.Conn, size uint32) error {
	toWrite := make([]byte, 4)
	binary.BigEndian.PutUint32(toWrite, size)
	return writeAll(conn, toWrite)
}

func readMessageSize(conn net.Conn) (uint32, error) {
	sizeBuff := make([]byte, 4)
	err := readAll(conn, sizeBuff)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(sizeBuff), nil
}
