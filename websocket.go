package kubexp

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func websocketExecutor(url string, token string) (string, error) {
	wsc, _, err := getWSConnection(url, token)
	if err != nil {
		return "", err
	}
	var respBuf bytes.Buffer
	for {
		_, message, err := wsc.ReadMessage()
		if err != nil {
			if err.Error() != "websocket: close 1000 (normal)" {
				errorlog.Printf("websocket read error: %v", err)
				return respBuf.String(), err
			}
			break
		}
		respBuf.Write(message)
	}
	return respBuf.String(), nil
}

func websocketConnect(url string, token string, closeCallback func()) (chan []byte, chan []byte, error) {
	wsc, _, err := getWSConnection(url, token)
	if err != nil {
		return nil, nil, err
	}

	in := make(chan []byte)
	out := make(chan []byte)
	closeChan := make(chan bool)

	go func() {
		defer wsc.Close()
		defer close(in)
		defer close(out)
		for {
			_, message, err := wsc.ReadMessage()
			if err != nil {
				closeChan <- true
				if err.Error() != "websocket: close 1000 (normal)" {
					errorlog.Fatalf("error from exec :%s", err)
				}
				time.Sleep(1000 * time.Millisecond)
				closeCallback()
				return
			}
			msg := message[1:]
			tracelog.Printf("out <- msg: %s", string(msg))
			out <- msg
		}
	}()

	go func() {
		for {
			select {
			case b := <-in:
				tracelog.Printf("write ws mesg: %s", string(b))
				wsc.WriteMessage(websocket.BinaryMessage, b)
			case <-closeChan:
				return
			}
		}
	}()
	return in, out, nil
}

func getWSConnection(url string, token string) (*websocket.Conn, *http.Response, error) {
	tracelog.Printf("ws connection url=%s", url)
	headers := make(http.Header)
	headers.Add("Authorization", "Bearer "+token)
	headers.Add("X-Stream-Protocol-Version", "channel.k8s.io")

	websocket.DefaultDialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	ws, rsp, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		errorlog.Printf("Can't create websocket for url:%s.\nresponse: %v\nerror: %v", url, rsp, err)
	}
	return ws, rsp, err
}
