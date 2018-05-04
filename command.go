package ftp

import (
	"bytes"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"strconv"
)

type command func(*Session, string)

var Commands map[string]command

// https://stackoverflow.com/questions/31726254/how-to-avoid-initialization-loop-in-go
func init() {
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
	log.Println("set login " + login)
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
	log.Println("set password " + password)
}

func quit(s *Session, _ string) {
	s.exit = true
	if s.dataConn != nil {
		close(s.data)
		s.dataConn.Close() //todo close to transfer func
	}
	s.response(221, "Goodbye.", false)
	log.Println("quit")
}

//todo parse addr
func createActiveConn(s *Session, addr string) {
	s.stopTransfer = false
	go func(s *Session, addr string) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log.Println("cannot connected to remote client")
			//todo client response
			return
		}
		s.dataConn = conn
		s.response(200, "PORT command successful.", false)
		log.Println("create active connection to " + addr)
	}(s, addr)
}

func createPassiveConn(s *Session, _ string) {
	s.stopTransfer = false
	go func(s *Session) {
		//todo what is the port?
		log.Println("start passive connection...")
		listener, err := net.Listen("tcp", "")
		if err != nil {
			log.Println("cannot listen port:", err.Error())
			return
		}
		//todo when to close the listener???
		//todo format address
		s.response(227, "Entering Passive Mode ("+listener.Addr().String()+")", false)
		log.Println("create passive connection to port", listener.Addr())
		conn, err := listener.Accept()
		if err != nil {
			log.Println("cannot connected to remote client", err.Error())
			return
		}
		s.dataConn = conn
	}(s)
}

//todo enum type
func setType(s *Session, transferType string) {
	s.transferType = transferType
	s.response(200, "Type set to "+transferType, false)
	log.Println("set type " + transferType)
}

func setMode(s *Session, mode string) {
	s.mode = mode
	//todo change response message
	s.response(200, "i'm teapot", false)
	log.Println("set mode " + mode)
}

func setStructure(s *Session, structure string) {
	s.structure = structure
	s.response(200, "i'm teapot", false)
	log.Println("set structure " + structure)
}

func sendFile(s *Session, pathname string) {
	s.stopTransfer = false
	s.data <- data{
		process: func(s *Session, file string) {
			//todo don't read all file to mem
			//todo get file size
			readFile, err := ioutil.ReadFile(s.currentDir + pathname)
			if err != nil {
				//todo improve err handle
				s.response(552, "Requested file action aborted.", false)
				log.Println("cannot read file:", err.Error())
				return
			}
			//todo dataConn != nil ?
			s.dataConn.Write(readFile)
			log.Println("send file " + file)
		},
		value: pathname,
	}
	//todo wrong message
	s.response(150, "Opening data connection", false)
	log.Println("start send file " + pathname)
}

func storeFile(s *Session, pathname string) {
	s.stopTransfer = false
	s.data <- data{
		process: func(s *Session, file string) {
			readFile, err := ioutil.ReadAll(s.dataConn)
			if err != nil {
				//todo improve err handle
				s.response(552, "Requested file action aborted.", false)
				return
			}
			ioutil.WriteFile(s.currentDir+file, readFile, 0)
		},
		value: pathname,
	}
	//todo wrong message
	s.response(150, "Opening data connection", false)
	log.Println("store file " + pathname)
}

func noop(s *Session, _ string) {
	s.response(200, "NOOP command successful.", false)
	log.Println("no-op")
}

func deleteFile(s *Session, pathname string) {
	//todo os.PathSeparator
	err := os.Remove(s.currentDir + pathname)
	if err != nil {
		//todo improve message (for any cases)
		s.response(521, "Removing file was failed.", false)
		return
	}
	s.response(250, "DELE command successful.", false)
	log.Println("delete file " + pathname)
}

func removeDir(s *Session, pathname string) {
	//todo check auth
	//todo correct perform current dir
	err := os.RemoveAll(s.currentDir + pathname)
	if err != nil {
		//todo improve message (for any cases)
		s.response(521, "Removing file was failed.", false)
		return
	}
	s.response(250, "RMD command successful.", false)
	log.Println("remove directory " + pathname)
}

func changeDir(s *Session, pathname string) {
	if pathname == "" {
		s.response(501, "Invalid number of parameters.", false)
		return
	}
	s.currentDir = s.resolveDir(pathname)
	//todo improve message
	s.response(250, "CWD command successful", false)
	log.Println("change dir to " + pathname)
}

//todo check not empty param if needed
func makeDir(s *Session, pathname string) {
	if pathname == "" {
		s.response(501, "Invalid number of parameters.", false)
		return
	}
	if err := os.Mkdir(s.resolveDir(pathname), 0); err == nil {
		//todo resolve pathname
		s.response(257, `"`+pathname+`" - Directory successfully created.`, false)
		log.Println("make dir " + pathname)
	} else {
		s.response(521, "Making directory was failed.", false)
		log.Println("ERROR: cannot make directory", err.Error())
	}
}

func printCurrentDir(s *Session, _ string) {
	s.response(257, `"`+s.clippedDir()+`" is current directory.`, false)
	log.Println("print current directory")
}

func getFiles(s *Session, pathname string) {
	var dir string
	if pathname == "" {
		dir = s.currentDir
	} else {
		dir = s.resolveDir(pathname)
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Println(err.Error())
		s.response(550, dir+": No such directory", false)
		return
	}

	log.Println("get all files in " + dir)
	//todo create data ASCII connection
	s.response(150, "Accepted data connection", false)
	if s.dataConn == nil {
		s.response(425, "Cannot open data connection.", false)
		//todo print connection infos (e.g. [ip:port])
		log.Println("WARN: data connection is empty")
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
		s.dataConn.Close()
	}
	s.response(226, "ABOR command successful", false)
	log.Println("cancel transfer")
}

func setFileForRename(s *Session, pathname string) {
	s.fileToRename = s.resolveDir(pathname)
	s.response(350, "Waiting for file name input.", false)
	log.Println("set file for rename " + pathname)
}

func renameFile(s *Session, newName string) {
	fileName := path.Base(newName)
	fileDir, oldFileName := path.Split(s.fileToRename)
	os.Rename(s.fileToRename, fileDir+fileName)
	s.response(250, `File "`+oldFileName+`" renamed to "`+fileName+`"`, false)
	log.Println("rename file to " + newName)
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
