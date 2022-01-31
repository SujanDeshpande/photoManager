package utils

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func CheckError(err error) {
	if err != nil {
		panic(err)
	}
}

func Quit(serviceName string, Close func()) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("Closing %s !!!", serviceName)
	Close()
}
