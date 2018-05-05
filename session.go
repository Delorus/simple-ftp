package ftp

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//todo change export/not export field
type Session struct {
	conn net.Conn
	id   string

	//todo do not save in mem
	login    string
	passHash string
	//todo add user permission
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
	value   string //todo to []byte
}

func transfer(s *Session, data <-chan data) {
	for v := range data {
		if s.dataConn == nil {
			continue
		}
		logInfo(s, "start transfer data")
		v.process(s, v.value)
		s.dataConn.Close()
	}
}

func NewSession(conn net.Conn) *Session {
	//todo for debug only
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err.Error())
	}
	cwd = filepath.ToSlash(cwd)

	hashSum := md5.Sum([]byte(conn.RemoteAddr().String() + time.Now().String()))
	result := &Session{
		conn: conn,
		id:   hex.EncodeToString(hashSum[:]),
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
	logResp(s, resp.String())
	resp.WriteTo(s.conn)
}

func (s *Session) resolveDir(rawDir string) string {
	if path.IsAbs(rawDir) {
		return s.rootDir + rawDir
	}
	dir := path.Join(s.currentDir, rawDir)
	if dir[0] == '/' {
		return s.rootDir + dir //todo attempt broken root path
	} else {
		return dir
	}
}

func (s *Session) clippedDir() string {
	clipDir := strings.TrimPrefix(s.currentDir, s.rootDir)
	if clipDir == "" {
		return "/"
	} else {
		return clipDir
	}
}

func toFtpAddr(addr string) (string, error) {
	rawAddr := strings.Split(addr, ":")

	ip := strings.Split(rawAddr[0], ".")

	port, err := strconv.Atoi(rawAddr[1])
	if err != nil {
		return "", fmt.Errorf("not supported address format: %s", addr)
	}
	first := port / 256
	second := port - (first * 256)

	ip = append(ip, strconv.Itoa(first))
	ip = append(ip, strconv.Itoa(second))

	return strings.Join(ip, ","), nil
}

//todo validate addr
func toTcpIpAddr(ftpAddr string) (string, error) {
	rawAddr := strings.Split(ftpAddr, ",")
	ip := strings.Join(rawAddr[:4], ".")

	first, err := strconv.Atoi(rawAddr[4])
	if err != nil {
		return "", fmt.Errorf("not supported address format: %s", ftpAddr)
	}
	second, err := strconv.Atoi(rawAddr[5])
	if err != nil {
		return "", fmt.Errorf("not supported address format: %s", ftpAddr)
	}

	return ip + strconv.Itoa(first*256+second), nil
}
