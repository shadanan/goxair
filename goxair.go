package main

import (
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
	address := c.Param("address")
	msg := xair.Get(address)
	c.JSON(200, gin.H{
		"address":   msg.Address,
		"arguments": msg.Arguments,
		"xair":      "XR18-5E-91-5A",
	})
}

func main() {
	xair = NewXAir("192.168.86.48:10024")
	go xair.Start()
	r := gin.Default()
	r.GET("/api/xairs", xairsGet)
	r.GET("/api/xairs/:xair/addresses/*address", oscGet)
	r.Run()
}
