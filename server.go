// Minimum implementation FTP-server
// see: http://www.ietf.org/rfc/rfc959.txt

package ftp

import (
	"bufio"
	"fmt"
	"log" //todo change logging
	"net"
	"strings"
)

func Run() {
	log.Println("start ftp server...")
	server, err := net.Listen("tcp", ":2121")
	if err != nil {
		log.Fatal("cannot start server:", err.Error())
	}
	defer server.Close()

	for {
		if conn, err := server.Accept(); err == nil {
			go handle(conn) //todo connection timeout
		} else {
			log.Println("WARN: connected lost:", err.Error())
			conn.Close()
		}
	}
}

func handle(conn net.Conn) {
	log.Println("start new connection", conn.RemoteAddr())
	session := NewSession(conn)
	session.response(220, "Simple FTP Server", false)

	reader := bufio.NewReader(conn)
	for {
		rawRequest, err := reader.ReadString('\n') //todo bug: not read message: "port XYZ"
		if err != nil {
			printfWarn("cannot read request", conn, err)
			return
		}

		req, err := parse(rawRequest)
		if err != nil {
			printfWarn("cannot parse request", conn, err)
			return
		}

		processRequest(session, req)

		if session.exit == true {
			break
		}
	}

	log.Println("close connection to", conn.RemoteAddr())
	conn.Close()
}

type request struct {
	cmdName string
	args    string
}

func parse(raw string) (request, error) {
	//todo just converting to string, for the time being...
	var result = request{}
	rawCommand := strings.SplitN(strings.TrimSpace(raw), " ", 2) //todo utf8?
	if len(rawCommand) < 1 {
		return result, fmt.Errorf("request is empty")
	}
	result.cmdName = strings.ToUpper(rawCommand[0])

	if len(rawCommand) > 1 {
		result.args = rawCommand[1]
	}
	return result, nil
}

func processRequest(s *Session, req request) {
	if command, ok := Commands[req.cmdName]; ok {
		command(s, req.args)
	} else {
		s.response(500, req.cmdName+" not understood.", false)
		log.Println("WARN: unknown command:", req.cmdName)
	}
}

func printfWarn(message string, conn net.Conn, err error) {
	log.Printf("[%15s] WARN: %s: %s", conn.RemoteAddr(), message, err)
}
