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
	"io/ioutil"
	"flag"
)

const ObjectShaLength = 20
const TruncatedSize = 3072

type objectHeader struct {
	objectType string
	length     int
}

type commitObject struct {
	tree string
	parent string
	author string
	commit_message string
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

func printCommitContent(bufScanner *bufio.Scanner, byteCount int) {
	//format:
	// tree <tree sha>
	// parent <parent sha>
	// [parent <parent sha> if several parents from merges]
	// author <author name> <author e-mail> <timestamp> <timezone>
	// committer <author name> <author e-mail> <timestamp> <timezone>

	// <commit message>

	fileMetadataBytes := scanCountBytes(bufScanner, byteCount, true)
	fileMetadataString := string(fileMetadataBytes)
	fmt.Print(fileMetadataString)
}

func printBlobContent(bufScanner *bufio.Scanner, byteCount int) {
	//format:
	// <content>
	// ...
	// print 3KB size (atmax) of object content
	var count int
	if (byteCount > TruncatedSize) {
		count = TruncatedSize
	} else {
		count = byteCount
	}
	fileMetadataBytes := scanCountBytes(bufScanner, count, true)
	fileMetadataString := string(fileMetadataBytes)
	fmt.Print(fileMetadataString)
	if (byteCount > TruncatedSize) {
		fmt.Printf("\n\n(... truncated to 3KB)\n")
	}
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

func printObjectFileContent(contentReader io.Reader) {
	bufScanner := bufio.NewScanner(contentReader)
	bufScanner.Split(bufio.ScanBytes) // read byte by byte
	header := parseObjectHeader(bufScanner)
	fmt.Printf("Type: %s, len: %d\n", header.objectType, header.length)
	if header.objectType == "tree" {
		printTreeContent(bufScanner)
	} else if header.objectType == "blob" {
		printBlobContent(bufScanner, header.length)
	} else if header.objectType == "commit" {
		printCommitContent(bufScanner, header.length)
	} else {
		fmt.Println("Parsing this tag-type not yet supported")
	}
}

func readFile(path string) string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	bufScanner := bufio.NewScanner(file)
	bufScanner.Split(bufio.ScanBytes) // read byte by byte
	fileMetadataBytes := scanBytesUntilDelimiter(bufScanner, byte('\n'), false)
	fileMetadataString := string(fileMetadataBytes[:len(fileMetadataBytes)-1])
	return fileMetadataString
}

func listBranches() {
	path := ".git/refs/heads"
	branches, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}
	current_branch := readFile(".git/HEAD")
	// current-branch format
	// .git/HEAD => ref: refs/heads/<branch-name>
	for _, branch := range branches {
		// format : each branch resides in path => .git/refs/heads/<branch-name>
		branch_hash := readFile(path + "/" + branch.Name())
		if branch.Name() == strings.Split(current_branch, "/")[2] {
			fmt.Println("* " + branch.Name() + " " + branch_hash)
		} else {
			fmt.Println(branch.Name() + " " + branch_hash)
		}
	}
}

func parseObjectFile(hash string) {
	path := ".git/objects/"
	objectFile, err := os.Open(path + hash[0:2] + "/" + hash[2:])
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

func main() {
	branch := flag.Bool("branch", false, "list all branches")
	hash := flag.String("hash", "", "hash of the object file")
	flag.Parse()
	if *branch == true { 	// git branch -l
		listBranches()
	} else if *hash != "" {		// git cat-file -p <hash>
		parseObjectFile(*hash)
	} else {
		fmt.Println("No flag selected.. Try --help|-h")
	}
}
