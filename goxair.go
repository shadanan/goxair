package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var scanner = NewScanner()

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func xairsGet(c *gin.Context) {
	c.JSON(200, gin.H{
		"xairs": scanner.List(),
	})
}

func xairsWs(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		Log.Warn.Printf("Failed to upgrade websocket: %+v", err)
		return
	}
	defer ws.Close()

	stopWebsocket := make(chan struct{})
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				break
			}
		}
		close(stopWebsocket)
	}()

	sub := scanner.Subscribe()
	defer scanner.Unsubscribe(sub)

	for {
		err := ws.WriteJSON(gin.H{
			"xairs": scanner.List(),
		})
		if err != nil {
			Log.Warn.Printf("Error writing json: %+v", err)
		}

		select {
		case <-stopWebsocket:
			return
		case <-sub:
			continue
		}
	}
}

func oscGet(c *gin.Context) {
	xair := scanner.xairs[c.Param("xair")]
	address := c.Param("address")

	msg, err := xair.Get(address)
	if errors.Is(err, ErrTimeout) {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("%s not found on %s", address, xair.name),
		})
		return
	}

	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
	})
}

func oscPatch(c *gin.Context) {
	xair := scanner.xairs[c.Param("xair")]

	data, err := c.GetRawData()
	if err != nil {
		panic(err.Error())
	}

	msg := Message{}
	json.Unmarshal(data, &msg)

	xair.Set(msg.Address, msg.Arguments)

	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
	})
}

func oscWs(c *gin.Context) {
	xair := scanner.xairs[c.Param("xair")]

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		Log.Warn.Printf("Failed to upgrade websocket: %+v", err)
		return
	}
	defer ws.Close()

	stopWebsocket := make(chan struct{})
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				break
			}
		}
		close(stopWebsocket)
	}()

	sub := xair.Subscribe()
	defer xair.Unsubscribe(sub)

	for {
		select {
		case <-stopWebsocket:
			return
		case msg := <-sub:
			err := ws.WriteJSON(gin.H{
				"address":   msg.Address,
				"arguments": msg.Arguments,
			})
			if err != nil {
				Log.Warn.Printf("Error writing json: %+v", err)
			}
		}
	}
}

func main() {
	go scanner.Start()
	defer scanner.Stop()

	r := gin.Default()
	r.GET("/api/xairs", xairsGet)
	r.GET("/ws/xairs", xairsWs)
	r.GET("/api/xairs/:xair/addresses/*address", oscGet)
	r.PATCH("/api/xairs/:xair/addresses/*address", oscPatch)
	r.GET("/ws/xairs/:xair/addresses", oscWs)

	r.Run()
}
