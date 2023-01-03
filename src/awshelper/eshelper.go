package awshelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/opensearch-project/opensearch-go/signer/aws"
)

// ESHelper is a helper for AWS Elasticsearch Service
type ESHelper struct {
	ESClient 	*opensearch.Client
	index 		string
}

// NewESHelper creates a new ESHelper
func NewESHelper(endpoint string, esIndex string) *ESHelper {
	// Get credentials from environment variables and create the Signature Version 4 signer
	sess := session.Must(session.NewSession())

	sessConfig := sess.Config
	signer, err := aws.NewSigner(session.Options{
		Config: *sessConfig,
	})
	if err != nil {
		panic(fmt.Sprintf("Could not create signer properly: %v" , err))
	}

	client, _ := opensearch.NewClient(opensearch.Config{
		Addresses: []string{endpoint},
		Signer:    signer,
		
	})

	if info, err := client.Info(); err != nil {
		log.Fatal("info", err)
	} else {
		var r map[string]interface{}
		json.NewDecoder(info.Body).Decode(&r)
		version := r["version"].(map[string]interface{})
		fmt.Printf("%s: %s\n", version["distribution"], version["number"])
		fmt.Println("Successfully established connection to ", endpoint)
	}

	return &ESHelper{
		ESClient: client,
		index: esIndex,
	}
}

func (es *ESHelper) PostBulk(documentId string, payload []byte) error {
	// Create the indexer
	//
	indexer, err := opensearchutil.NewBulkIndexer(opensearchutil.BulkIndexerConfig{
		Client:     es.ESClient, // The OpenSearch client
		Index:      es.index, // The default index name
	})
	if err != nil {
		log.Fatalf("Error creating the indexer: %s", err)
		return err
	}

	// Add an item to the indexer
	//
	err = indexer.Add(
		context.Background(),
		opensearchutil.BulkIndexerItem{
			// Action field configures the operation to perform (index, create, delete, update)
			Action: "index",

			// DocumentID is the optional document ID
			DocumentID: documentId,

			// Body is an `io.Reader` with the payload
			Body: bytes.NewReader(payload),

			// OnSuccess is the optional callback for each successful operation
			OnSuccess: func(
				ctx context.Context,
				item opensearchutil.BulkIndexerItem,
				res opensearchutil.BulkIndexerResponseItem,
			) {
				fmt.Printf("[%d] %s test/%s", res.Status, res.Result, item.DocumentID)
			},

			// OnFailure is the optional callback for each failed operation
			OnFailure: func(
				ctx context.Context,
				item opensearchutil.BulkIndexerItem,
				res opensearchutil.BulkIndexerResponseItem, err error,
			) {
				if err != nil {
					log.Printf("ERROR: %s for payload %s \n", err, string(payload))
					log.Printf("ERROR: %s: %s", res.Error.Type, res.Error.Reason)
				} else {
					log.Printf("ERROR for payload %s %s: %s \n", string(payload), res.Error.Type, res.Error.Reason)
				}
			},
		},
	)
	if err != nil {
		log.Fatalf("Unexpected error: %s", err)
		return err
	}

	// Close the indexer channel and flush remaining items
	//
	if err := indexer.Close(context.Background()); err != nil {
		log.Fatalf("Unexpected error closing the channel: %s", err)
	}

	// Report the indexer statistics
	//
	stats := indexer.Stats()
	if stats.NumFailed > 0 {
		log.Fatalf("Indexed [%d] documents with [%d] errors", stats.NumFlushed, stats.NumFailed)
	} else {
		log.Printf("Successfully indexed [%d] documents", stats.NumFlushed)
	}

	return nil
}


