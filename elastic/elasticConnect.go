package elastic

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	elastic "github.com/olivere/elastic/v7"
)

var Client *elastic.Client

func InitializeElastic() {
	client, err := elastic.NewClient(elastic.SetURL("http://localhost:9200"),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false))
	if err != nil {
		fmt.Println("error", err)
	} else {
		fmt.Println("Connected")
	}
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("Closing Yugabyte")
	}()
	fmt.Println("Started Elastic")
	Client = client
}
