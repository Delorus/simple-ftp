package ftp

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
)

type command func(*Session, string)

var Commands map[string]command

// https://stackoverflow.com/questions/31726254/how-to-avoid-initialization-loop-in-go
func init() {
	//todo to switch-case?
	Commands = map[string]command{
		//todo sort
		"USER": setLogin,
		"PASS": setPassword,
		"QUIT": quit,
		"PORT": createActiveConn, //todo wtf not working from telnet with params, e.g. "port 123"
		"PASV": createPassiveConn,
		"TYPE": setType,
		"MODE": setMode,
		"STRU": setStructure,
		"RETR": sendFile,
		"STOR": storeFile,
		"NOOP": noop,
		"DELE": deleteFile,
		"RMD":  removeDir,
		"CWD":  changeDir,
		"MKD":  makeDir,
		"PWD":  printCurrentDir,
		"LIST": getFiles,
		"ABOR": abortTransfer,
		"RNFR": setFileForRename,
		"RNTO": renameFile,
		"HELP": help,
	}
}

func setLogin(s *Session, login string) {
	//todo login is not empty
	//todo anonymous login
	s.login = login
	s.response(331, "Password required for "+login, false)
}

//todo add anonymous
//todo add protection from bruteforce
//todo check auth
func setPassword(s *Session, password string) {
	if s.login == "" {
		s.response(503, "Bad sequence of commands. Send USER first.", false)
		return
	}
	s.passHash = password //todo hash
	s.response(230, "User "+s.login+" logged in.", false)
}

func quit(s *Session, _ string) {
	s.exit = true
	if s.dataConn != nil {
		close(s.data)
		s.dataConn.Close() //todo close to transfer func
	}
	s.response(221, "Goodbye.", false)
}

func createActiveConn(s *Session, addr string) {
	s.stopTransfer = false
	go func(s *Session, addr string) {
		logInfo(s, "start active connection to addr:", addr)
		tcpAddr, err := toTcpIpAddr(addr)
		if err != nil {
			logErr(s, err.Error())
			return
		}
		conn, err := net.Dial("tcp", tcpAddr)
		if err != nil {
			logErr(s, "cannot connected to remote client", err.Error())
			//todo client response
			return
		}
		s.dataConn = conn
		s.response(200, "PORT command successful.", false)
	}(s, addr)
}

func createPassiveConn(s *Session, _ string) {
	s.stopTransfer = false
	go func(s *Session) {
		logInfo(s, "start passive connection")
		//todo what is the port?
		//todo use not only default ip (0,0,0,0)
		listener, err := net.Listen("tcp4", "")
		if err != nil {
			logErr(s, "cannot listen port:", err.Error())
			return
		}
		//todo when to close the listener???
		//todo format address
		ftpAddr, err := toFtpAddr(listener.Addr().String())
		if err != nil {
			logErr(s, err.Error())
			return
		}
		s.response(227, "Entering Passive Mode ("+ftpAddr+")", false)

		conn, err := listener.Accept()
		if err != nil {
			logErr(s, "cannot connected to remote client", err.Error())
			return
		}
		s.dataConn = conn
	}(s)
}

//todo enum type
func setType(s *Session, transferType string) {
	s.transferType = transferType
	s.response(200, "Type set to "+transferType, false)
}

func setMode(s *Session, mode string) {
	s.mode = mode
	//todo change response message
	s.response(200, "i'm teapot", false)
}

func setStructure(s *Session, structure string) {
	s.structure = structure
	s.response(200, "i'm teapot", false)
}

func sendFile(s *Session, pathname string) { //todo measure the transfer speed
	logInfo(s, "start send file", s.currentDir+"/"+pathname)
	s.stopTransfer = false
	s.data <- data{
		process: func(s *Session, file string) {
			logInfo(s, "send file", file)
			//todo get file size
			openedFile, err := os.Open(s.currentDir + "/" + file)
			if err != nil {
				//todo improve err handle
				s.response(552, "Requested file action aborted.", false)
				logErr(s, err.Error())
				return
			}
			defer openedFile.Close()

			//todo dataConn != nil ?
			pipe := io.TeeReader(openedFile, s.dataConn)
			if _, err = ioutil.ReadAll(pipe); err != nil {
				//todo improve err handle
				s.response(552, "Requested file action aborted.", false)
				logErr(s, err.Error())
				return
			}

			s.response(226, "Transfer complete.", false)
		},
		value: pathname,
	}
	//todo wrong message
	s.response(150, "Opening data connection", false)
}

func storeFile(s *Session, pathname string) {
	logInfo(s, "store file", s.currentDir+"/"+pathname)
	s.stopTransfer = false
	s.data <- data{
		process: func(s *Session, file string) {
			//todo don't read all file to mem
			readFile, err := ioutil.ReadAll(s.dataConn)
			if err != nil {
				//todo improve err handle
				s.response(552, "Requested file action aborted.", false)
				logErr(s, err.Error())
				return
			}
			ioutil.WriteFile(s.currentDir+"/"+file, readFile, 0) //todo perm?
			s.response(226, "Transfer complete.", false)
		},
		value: pathname,
	}
	//todo wrong message
	s.response(150, "Opening data connection", false)
}

func noop(s *Session, _ string) {
	s.response(200, "NOOP command successful.", false)
}

func deleteFile(s *Session, pathname string) {
	logInfo(s, "delete file", s.currentDir+"/"+pathname)
	//todo os.PathSeparator
	err := os.Remove(s.currentDir + "/" + pathname)
	if err != nil {
		//todo improve message (for any cases)
		s.response(521, "Removing file was failed.", false)
		logErr(s, err.Error())
		return
	}
	s.response(250, "DELE command successful.", false)
}

func removeDir(s *Session, pathname string) {
	logInfo(s, "delete directory", s.currentDir+"/"+pathname)
	//todo check auth
	//todo correct perform current dir
	err := os.RemoveAll(s.currentDir + "/" + pathname)
	if err != nil {
		//todo improve message (for any cases)
		s.response(521, "Removing file was failed.", false)
		logErr(s, err.Error())
		return
	}
	s.response(250, "RMD command successful.", false)
}

func changeDir(s *Session, pathname string) {
	logInfo(s, "change dir to", pathname)
	if pathname == "" {
		s.response(501, "Invalid number of parameters.", false)
		return
	}
	s.currentDir = s.resolveDir(pathname)
	//todo improve message
	s.response(250, "CWD command successful", false)
	logInfo(s, "current dir:", s.currentDir)
}

//todo check not empty param if needed
func makeDir(s *Session, pathname string) {
	logInfo(s, "create directory:", pathname)
	if pathname == "" {
		s.response(501, "Invalid number of parameters.", false)
		return
	}
	if err := os.Mkdir(s.resolveDir(pathname), 0); err == nil {
		//todo resolve pathname
		s.response(257, `"`+pathname+`" - Directory successfully created.`, false)
	} else {
		s.response(521, "Making directory was failed.", false)
		logErr(s, err.Error())
	}
}

func printCurrentDir(s *Session, _ string) {
	s.response(257, `"`+s.clippedDir()+`" is current directory.`, false)
}

func getFiles(s *Session, pathname string) {
	var dir string
	if pathname == "" {
		dir = s.currentDir
	} else {
		dir = s.resolveDir(pathname)
	}
	logInfo(s, "send list of files in dir:", dir)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		s.response(550, dir+": No such directory", false)
		logErr(s, err.Error())
		return
	}

	//todo create data ASCII connection
	s.response(150, "Accepted data connection", false)
	if s.dataConn == nil {
		s.response(425, "Cannot open data connection.", false)
		return
	}

	//todo in new goroutine
	//todo format output to RFC959
	var buf bytes.Buffer
	for _, file := range files {
		buf.WriteString(file.Name() + "\n")
	}
	s.stopTransfer = false
	s.data <- data{
		process: func(s *Session, list string) {
			//todo send list options
			s.response(226, strconv.Itoa(len(files))+" matches total", false)
			s.dataConn.Write([]byte(list))
		},
		value: buf.String(),
	}
}

func abortTransfer(s *Session, _ string) {
	s.stopTransfer = true
	if s.dataConn != nil {
		s.dataConn.Close() //todo bug: write error in log
		logInfo(s, "data connection closed")
	}
	s.response(226, "ABOR command successful", false)
}

func setFileForRename(s *Session, pathname string) {
	s.fileToRename = s.resolveDir(pathname)
	logInfo(s, "rename file:", s.fileToRename)
	s.response(350, "Waiting for file name input.", false)
}

func renameFile(s *Session, newName string) {
	fileName := path.Base(newName)
	fileDir, oldFileName := path.Split(s.fileToRename)
	os.Rename(s.fileToRename, fileDir+fileName)
	s.response(250, `File "`+oldFileName+`" renamed to "`+fileName+`"`, false)
}

func help(s *Session, _ string) {
	s.response(214, "The following commands are recognized", true)
	var buf bytes.Buffer
	for k := range Commands {
		buf.WriteString("    ")
		buf.WriteString(k)
		buf.WriteRune('\n')
	}
	buf.WriteTo(s.conn)
	s.response(214, "HELP command successful.", false)
}
