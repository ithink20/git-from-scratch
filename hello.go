package main

import (
	"bufio"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

const ObjectShaLength = 20

type objectHeader struct {
	objectType string
	length     int
}

func scanSingleByte(bufScanner *bufio.Scanner, throwOnEOF bool) (byte, bool) {
	readSuccess := bufScanner.Scan()
	if !readSuccess {
		err := bufScanner.Err()
		if err != nil {
			log.Fatal(err)
		} else {
			// found EOF
			if throwOnEOF {
				panic("Unexpected EOF")
			}
			return 0, true
		}
	}
	return bufScanner.Bytes()[0], false
}

func scanBytesUntilDelimiter(bufScanner *bufio.Scanner, delimiter byte, throwOnEOF bool) []byte {
	// delimiter is scanned and appended in the return value
	scannedBytes := make([]byte, 0)
	for {
		byteRead, isFailure := scanSingleByte(bufScanner, throwOnEOF)
		if isFailure {
			return scannedBytes
		}
		scannedBytes = append(scannedBytes, byteRead)
		if byteRead == delimiter {
			return scannedBytes
		}
	}
}

func scanCountBytes(bufScanner *bufio.Scanner, byteCount int, throwOnEOF bool) []byte {
	scannedBytes := make([]byte, 0)
	for i := 1; i <= byteCount; i++ {
		byteRead, _ := scanSingleByte(bufScanner, true)
		scannedBytes = append(scannedBytes, byteRead)
	}
	return scannedBytes
}

func parseObjectHeader(bufScanner *bufio.Scanner) objectHeader {
	// header format: "<object-type-string> <length-in-string>\0"
	headerBytes := scanBytesUntilDelimiter(bufScanner, 0, true) // '\0' character in ascii is same as 0
	headerString := string(headerBytes[:len(headerBytes)-1])
	headerComponents := strings.Split(headerString, " ")
	if len(headerComponents) != 2 {
		panic(fmt.Sprintf("Invalid header: %s", headerComponents))
	}
	objectLen, err := strconv.Atoi(headerComponents[1])
	if err != nil {
		panic(err)
	}
	return objectHeader{headerComponents[0], objectLen}
}

func printTreeContent(bufScanner *bufio.Scanner) {
	// format:
	// <file-mode-in-string> <file-name>\0<20-bytes-of-hash-in-binary>
	// <file-mode-in-string> <file-name>\0<20-bytes-of-hash-in-binary>
	// ...
	for {
		fileMetadataBytes := scanBytesUntilDelimiter(bufScanner, 0, false)
		if len(fileMetadataBytes) == 0 {
			// end of tree contents
			return
		}
		fileMetadataBytesLen := len(fileMetadataBytes)
		if fileMetadataBytes[fileMetadataBytesLen-1] != 0 {
			panic("Unexpected end of file-metadata")
		}
		fileMetadataBytes = fileMetadataBytes[:fileMetadataBytesLen-1] // remove trailing '\0'
		fileMetadataString := string(fileMetadataBytes)
		fileMetadataComponents := strings.Split(fileMetadataString, " ")
		if len(fileMetadataComponents) != 2 {
			panic("fileMetadataComponents len must be 2")
		}
		objectShaBytes := scanCountBytes(bufScanner, ObjectShaLength, true)
		objectShaString := hex.EncodeToString(objectShaBytes)
		fmt.Printf("fileMode: %s, filename: %s, SHA: %s\n", fileMetadataComponents[0], fileMetadataComponents[1], objectShaString)
	}
}

func printBlobContent(bufScanner *bufio.Scanner) {
	//format:
	// <content>
    // ...
	for {
		fileMetadataBytes := scanBytesUntilDelimiter(bufScanner, 0, false)
		if len(fileMetadataBytes) == 0 {
			// end of blob contents
			return
        }
		fileMetadataString := string(fileMetadataBytes)
		fmt.Print(fileMetadataString)
	}
}

func printObjectFileContent(contentReader io.Reader) {
	bufScanner := bufio.NewScanner(contentReader)
	bufScanner.Split(bufio.ScanBytes) // read byte by byte
	header := parseObjectHeader(bufScanner)
	fmt.Printf("Type: %s, len: %d\n", header.objectType, header.length)
	if header.objectType == "tree" {
		printTreeContent(bufScanner)
	} else if header.objectType == "blob" {
		printBlobContent(bufScanner)
	} else {
		fmt.Println("Parsing this blob-type not yet supported")
	}
}

func main() {
	path := os.Args[1]
	objectFile, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	contentReader, err := zlib.NewReader(objectFile)
	if err != nil {
		log.Fatal(err)
	}
	printObjectFileContent(contentReader)
	contentReader.Close() // close reader when done
}
