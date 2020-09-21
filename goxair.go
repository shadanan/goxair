package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shadanan/goxair/log"
	"github.com/shadanan/goxair/osc"
	"github.com/shadanan/goxair/scanner"
	"github.com/shadanan/goxair/xair"
)

var scan = scanner.NewScanner(10)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func getXAir(c *gin.Context) (xair.XAir, bool) {
	xa, ok := scan.Get(c.Param("xair"))
	if !ok {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("XAir %s not found", c.Param("xair")),
		})
	}
	return xa, ok
}

func xairsGet(c *gin.Context) {
	c.JSON(200, gin.H{
		"xairs": scan.List(),
	})
}

func xairsWs(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error.Printf("Failed to upgrade websocket: %+v", err)
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

	sub := scan.Subscribe()
	defer scan.Unsubscribe(sub)

	for {
		err := ws.WriteJSON(gin.H{
			"xairs": scan.List(),
		})
		if err != nil {
			log.Error.Printf("Error writing json: %+v", err)
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
	xa, ok := getXAir(c)
	if !ok {
		return
	}

	address := c.Param("address")

	msg, err := xa.Get(address)
	if errors.Is(err, xair.ErrTimeout) {
		c.JSON(404, gin.H{
			"error": fmt.Sprintf("%s not found on %s", address, xa.Name),
		})
		return
	}

	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
	})
}

func oscPatch(c *gin.Context) {
	xa, ok := getXAir(c)
	if !ok {
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		panic(err.Error())
	}

	msg := osc.Message{}
	json.Unmarshal(data, &msg)

	xa.Set(msg.Address, msg.Arguments)

	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
	})
}

func oscWs(c *gin.Context) {
	xa, ok := getXAir(c)
	if !ok {
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error.Printf("Failed to upgrade websocket: %+v", err)
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

	sub := xa.Subscribe()
	defer xa.Unsubscribe(sub)

	for {
		select {
		case <-stopWebsocket:
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}

			err := ws.WriteJSON(gin.H{
				"address":   msg.Address,
				"arguments": msg.Arguments,
			})
			if err != nil {
				log.Error.Printf("Error writing json: %+v", err)
			}
		}
	}
}

func static(r *gin.Engine, html string) {
	files, err := ioutil.ReadDir(html)
	if err != nil {
		panic(err.Error())
	}

	for _, file := range files {
		path := fmt.Sprintf("%s/%s", html, file.Name())
		target := fmt.Sprintf("/%s", file.Name())
		if file.Name() == "index.html" {
			r.StaticFile("/", path)
		} else if file.IsDir() {
			r.Static(target, path)
		} else {
			r.StaticFile(target, path)
		}
	}
}

func main() {
	html := flag.String("html", "",
		"folder containing static files to serve")
	port := flag.Int("port", 8000,
		"the port to launch the xair proxy on")
	timeout := flag.Duration("timeout", 10*time.Second,
		"seconds to wait before marking XAir stale")
	flag.Parse()

	r := gin.Default()
	if *html != "" {
		static(r, *html)
	}

	r.GET("/api/xairs", xairsGet)
	r.GET("/ws/xairs", xairsWs)
	r.GET("/api/xairs/:xair/addresses/*address", oscGet)
	r.PATCH("/api/xairs/:xair/addresses/*address", oscPatch)
	r.GET("/ws/xairs/:xair/addresses", oscWs)

	go scan.Start(*timeout)
	defer scan.Stop()

	r.Run(fmt.Sprintf(":%d", *port))
}
