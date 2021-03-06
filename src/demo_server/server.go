package main

import "os"
import "fmt"
import "rudp"
import "time"
import "github.com/woodywanghg/gofclog"
import "github.com/woodywanghg/goini"
import "net/http"
import _ "net/http/pprof"

type TestServer struct {
}

func (t *TestServer) OnSessionCreate(sessionId int64, code int) {
	fmt.Printf("OnSessionCreate  code=%d\n", code)
}

func (t *TestServer) OnRecv(sessionId int64, b []byte) {

	fmt.Printf("OnRecv data len=%d\n", len(b))
}

func (t *TestServer) OnSessionError(sessionId int64, errCode int) {
	fmt.Printf("OnRecv data session id=%d, code=%d\n", sessionId, errCode)
}

func main() {

	go func() {
		fmt.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	fclog.Init(true, true, "rudp.log", 1048576, fclog.LEVEL_DEBUG)

	var iniObj goini.IniFile
	if !iniObj.Init("./server.ini") {
		os.Exit(0)
		return
	}

	serverIp := iniObj.ReadString("SERVER", "ip", "error")
	serverPort := iniObj.ReadInt("SERVER", "port", -1)
	statAddr := iniObj.ReadString("STAT", "addr", "error")

	var obj = rudp.GetReliableUdp()
	obj.Init()

	if serverIp != "error" && serverPort != -1 {
		err := obj.Listen("0.0.0.0", serverPort)

		if err != nil {
			fmt.Printf("Init server error! err=%s\n", err.Error())
			return
		}
	}

	obj.Stat(statAddr)

	var objTest TestServer
	obj.SetUdpInterface(&objTest)

	for {
		time.Sleep(1000000 * 5000)
		//TODO
	}
}
