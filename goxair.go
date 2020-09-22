package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	limit "github.com/aviddiviner/gin-limit"
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

	msg, hit, err := xa.Get(address)
	c.Set("msg", msg)
	c.Set("hit", hit)

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
	c.Set("msg", msg)
	c.Set("hit", false)

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

func static(r *gin.RouterGroup, html string) {
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

func oscLogFormatter(param gin.LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}

	if param.Latency > time.Minute {
		param.Latency = param.Latency - param.Latency%time.Second
	}

	cached := "-"
	if param.Keys["hit"].(bool) {
		cached = "!"
	}

	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %s -%s> %s\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		param.Path,
		cached,
		param.Keys["msg"],
		param.ErrorMessage,
	)
}

func main() {
	html := flag.String("html", "",
		"folder containing static files to serve")
	port := flag.Int("port", 8000,
		"the port to launch the xair proxy on")
	timeout := flag.Duration("timeout", 1*time.Second,
		"seconds to wait before marking XAir stale")
	flag.Parse()

	r := gin.New()
	r.Use(gin.Recovery())

	all := r.Group("/")
	all.Use(gin.Logger())

	if *html != "" {
		static(all, *html)
	}

	all.GET("/api/xairs", xairsGet)
	all.GET("/ws/xairs", xairsWs)
	all.GET("/ws/xairs/:xair/addresses", oscWs)

	osc := r.Group("/api/xairs/:xair/addresses")
	osc.Use(gin.LoggerWithFormatter(oscLogFormatter))
	osc.Use(limit.MaxAllowed(2))
	osc.GET("/*address", oscGet)
	osc.PATCH("/*address", oscPatch)

	go scan.Start(*timeout)
	defer scan.Stop()

	r.Run(fmt.Sprintf(":%d", *port))
}
