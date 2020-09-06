package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var xair XAir

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func xairsGet(c *gin.Context) {
	xairs := []string{}
	c.JSON(200, gin.H{
		"xairs": xairs,
	})
}

func oscGet(c *gin.Context) {
	xairName := c.Param("xair")
	address := c.Param("address")

	msg, err := xair.Get(address)
	if errors.Is(err, ErrTimeout) {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("%s not found on %s", address, xairName),
		})
		return
	}

	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
		"xair":      xair.name,
	})
}

func oscWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Log.Warn.Printf("failed to upgrade websocket: %+v", err)
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
				"xair":      xair.name,
			})
			if err != nil {
				Log.Warn.Printf("error writing json: %+v", err)
			}
		}
	}
}

func main() {
	xair = NewXAir("192.168.86.98:10024", "XR18-5E-91-5A", []int{2, 3, 5})
	go xair.Start()

	r := gin.Default()
	r.GET("/api/xairs", xairsGet)
	r.GET("/api/xairs/:xair/addresses/*address", oscGet)
	r.GET("/ws/xairs/:xair/addresses", func(c *gin.Context) {
		oscWs(c.Writer, c.Request)
	})
	r.Run()
}
