package ftp

import (
	"bytes"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

//todo change export/not export field
type Session struct {
	conn net.Conn

	//todo do not save in mem
	login    string
	passHash string
	//todo session time out

	connType     string //todo type
	dataConn     net.Conn
	data         chan data //todo type
	stopTransfer bool
	transferType string //todo type
	mode         string //todo type
	structure    string //todo type

	//todo default value
	rootDir    string //todo type
	currentDir string //todo type

	fileToRename string

	exit bool
}

type data struct {
	process command
	value   string
}

func transfer(s *Session, data <-chan data) {
	for v := range data {
		log.Println("start transfer data")
		v.process(s, v.value)
		//todo ABOR 226 the operation was canceled successfully
		//todo close connection after transfer finish
	}
	log.Println("start file transfer")
}

func NewSession(conn net.Conn) *Session {
	//todo for debug only
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err.Error())
	}
	cwd = filepath.ToSlash(cwd)
	result := &Session{
		conn: conn,
		data: make(chan data, 5), //todo send message if queue is full
		//todo other default value

		rootDir:    cwd,
		currentDir: cwd,
	}
	go transfer(result, result.data)
	return result
}

func (s *Session) response(code int, msg string, isPrefix bool) {
	var resp bytes.Buffer
	resp.WriteString(strconv.Itoa(code))
	if isPrefix {
		resp.WriteString("-")
	} else {
		resp.WriteString(" ")
	}
	resp.WriteString(msg + "\n")
	resp.WriteTo(s.conn)
}

func (s *Session) resolveDir(rawDir string) string {
	if path.IsAbs(rawDir) {
		return s.rootDir + rawDir
	}
	dir := path.Join(s.currentDir, rawDir)
	if dir[0] == '/' {
		return s.rootDir + dir
	} else {
		return dir
	}
}

func (s *Session) clippedDir() string {
	//todo lost root '/'
	clipDir := strings.TrimPrefix(s.currentDir, s.rootDir)
	if clipDir == "" {
		return "/"
	} else {
		return clipDir
	}
}
