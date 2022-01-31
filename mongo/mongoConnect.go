package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Collection *mongo.Collection

func InitializeMongo() {
	connectUri := "mongodb://localhost:6565"
	fmt.Println("Hello Mongo")
	fmt.Println(connectUri)
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(connectUri))
	if err != nil {
		fmt.Println("error", err)
	} else {
		fmt.Println("Connected")
	}
	coll := client.Database("photostore").Collection("files")
	var result bson.M
	coll.FindOne(context.TODO(), bson.D{}).Decode(&result)
	if err != nil {
		fmt.Println("error", err)
	}
	jsonData, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", jsonData)

	fmt.Println("End Mongo")

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("Closing Yugabyte")
		client.Disconnect(context.TODO())
	}()
	Collection = coll
}
