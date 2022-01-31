package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"photoManager/handle"
	"photoManager/postgres"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// CarrierPayload Carrier Payload
type FileInfo struct {
	name          string
	size          int64
	modifiedAt    time.Time
	modifiedAtStr string
}

func main() {
	//mongo.InitializeMongo()
	// elastic.InitializeElastic()
	postgres.InitializePostgres()
	ps := postgres.NewPhotoStore()
	postgres.Insert(ps)
	psList, _ := postgres.ReadAll()
	for _, v := range psList {
		fmt.Println("", v)
	}
	postgres.Update(1, "temp 3")
	psList, _ = postgres.ReadAll()
	for _, v := range psList {
		fmt.Println("", v)
	}
	postgres.Delete(1)
	psList, _ = postgres.ReadAll()
	for _, v := range psList {
		fmt.Println("", v)
	}

	fileProcessing()
	router := mux.NewRouter()
	handle.InitializeRoutes(router)
	srv := &http.Server{
		Addr:    "localhost:7878",
		Handler: router,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Failed to start http path listen: %s\n", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	// Need to define all client close
	logrus.Info("Shutting down Server ...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logrus.Fatalf("Server Shutdown: %s\n", err)
	}
	logrus.Infof("Server exiting")

	fmt.Println("Stopped Colissomo V2 server")

}

func fileProcessing() {
	destFolder := "/Users/sujandeshpande/test/dest1/"
	srcFolder := "/Users/sujandeshpande/test/"
	fmt.Println("Starting", destFolder)

	if _, err := os.Stat(destFolder); os.IsNotExist(err) {
		_ = os.Mkdir(destFolder, os.ModePerm)

	}

	argsWithoutProg := os.Args[1:]
	fmt.Println(argsWithoutProg)

	fileInfoSlice := make([]FileInfo, 1)
	filepath.Walk(srcFolder,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			modTime := info.ModTime()
			fileInfo := FileInfo{info.Name(), info.Size(), modTime, modTime.Format("2006-January-02")}
			ypath := destFolder + strconv.Itoa(modTime.Year())
			if _, err := os.Stat(ypath); os.IsNotExist(err) {
				_ = os.Mkdir(ypath, os.ModePerm)

			}

			mpath := ypath + "/" + modTime.Month().String()
			if _, err := os.Stat(mpath); os.IsNotExist(err) {
				_ = os.Mkdir(mpath, os.ModePerm)

			}

			if !info.IsDir() {
				os.Open(srcFolder + info.Name())
				existingFile, erre := os.Open(srcFolder + info.Name())
				CopyFile, errc := os.Create(mpath + "/" + info.Name())
				len, err := io.Copy(CopyFile, existingFile)
				if erre != nil {
					fmt.Println("erre --> ", info.Name(), err.Error(), len)
				}
				if errc != nil {
					fmt.Println("errc --> ", info.Name(), err.Error(), len)
				}
				if err != nil {
					fmt.Println("Unable to copy file --> ", info.Name(), err.Error(), len)
				}
				fmt.Println(" copied successfully", info.Name())

				existingFile.Close()
				CopyFile.Close()
				fileInfoSlice = append(fileInfoSlice, fileInfo)
			}

			return nil
		})

	printProcessedFiles(fileInfoSlice)
}

func printProcessedFiles(fileInfos []FileInfo) {
	for _, fileInfo := range fileInfos {
		fmt.Println(fileInfo.name, " --- ", fileInfo.modifiedAtStr)
	}
}
