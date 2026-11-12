// Command gateway is the public HTTP API for SPHEREx Sky Explorer. See docs/SPEC-gateway.md.
package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// newRouter builds the HTTP router. Kept separate from main so tests can
// exercise routes without binding a port.
func newRouter() *gin.Engine {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

func main() {
	if err := newRouter().Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
