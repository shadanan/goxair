package main

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
)

var xair XAir

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
		"xair":      "XR18-5E-91-5A",
	})
}

func main() {
	xair = NewXAir("192.168.86.46:10024")
	go xair.Start()
	r := gin.Default()
	r.GET("/api/xairs", xairsGet)
	r.GET("/api/xairs/:xair/addresses/*address", oscGet)
	r.Run()
}
