package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hellofresh/hfkafka/v2"
	"github.com/hellofresh/hfkafka/v2/kore"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
	"log"
	"os"
	"sync/atomic"
	"time"
)

var (
	server = "localhost:9092"
	topic  = "poc-test-part-1"

	seedSize = 1000
	workers  = 10

	dbName       = "test"
	dbCollection = "messages"
)

func main() {
	kConsumer, err := newConsumer()
	if err != nil {
		log.Fatal("failed to create kafka consumer")
	}

	mongoClient, err := newMongoDB()
	if err != nil {
		log.Fatal("failed to create mongoDB client")
	}

	ctx := context.TODO()

	collection := mongoClient.Database(dbName).Collection(dbCollection)
	err = collection.Drop(ctx) //reset state
	if err != nil {
		log.Fatal(fmt.Errorf("mongoDB: %w", err))
	}

	resultChannel := make(chan bool)
	var count int64

	c, err := hfkafka.NewConsumer(kConsumer, func(ctx context.Context, msg kore.ConsumedMessage) error {
		var m message
		err := json.Unmarshal(msg.Body, &m)
		if err != nil {
			log.Fatal(errors.Wrap(err, "JSON issue:"))
		}
		m.ConsumedAt = time.Now().UnixMicro()

		_, err = collection.InsertOne(ctx, m)
		if err != nil {
			log.Fatal(errors.Wrap(err, "Mongo issue:"))
		}

		atomic.AddInt64(&count, 1)
		if count == int64(seedSize) {
			resultChannel <- true
		}

		return nil
	},
		hfkafka.WithWorkers(workers))
	if err != nil {
		log.Fatal("failure starting kafka consumer", zap.Error(err))
	}

	go func() {
		err = c.Start(ctx)
		if err != nil {
			log.Fatal("failure starting kafka consumer", zap.Error(err))
			return
		}
	}()

	//Wait all messages
	<-resultChannel
	err = kConsumer.Close()
	if err != nil {
		log.Fatal("failed to close kafka consumer")
	}

	opt := options.Find()
	opt.SetLimit(int64(seedSize))
	opt.SetSort(bson.D{{"consumed_at", 1}})
	cur, err := collection.Find(context.TODO(), bson.D{{}}, opt)
	if err != nil {
		log.Fatal(err)
	}

	messages := make([]message, 0, seedSize)
	for cur.Next(ctx) {
		var m message
		err := cur.Decode(&m)
		if err != nil {
			log.Fatal(err)
		}
		messages = append(messages, m)
	}

	for i := 0; i < seedSize-1; i++ {
		if messages[i].PublishedAt > messages[i+1].PublishedAt {
			log.Fatal("wrong order")
		}
	}
	log.Fatal("success")
}

func newConsumer() (*kore.Consumer, error) {
	os.Setenv("KAFKA_DSN", server)
	os.Setenv("KAFKA_SECURITY_PROTOCOL", "PLAINTEXT")
	kConfig, err := kore.LoadEnv()

	if err != nil {
		return nil, err
	}

	kConfig.Consumer.Topics = []string{topic}
	kConfig.Consumer.GroupID = uuid.New().String() //Too lazy to reset offset manually
	kConfig.Consumer.ClientID = "hf-poc"
	kConfig.Consumer.EnableAutoCommit = false
	kConfig.Consumer.AutoOffsetReset = "earliest"
	kConfig.Consumer.MaxPollIntervalMS = 10000
	kConfig.Consumer.Batch = 100

	kConsumer, err := kore.NewConsumer(kConfig)

	if err != nil {
		return nil, err
	}

	return kConsumer, nil
}

func newMongoDB() (*mongo.Client, error) {
	var cred options.Credential
	cred.Username = "root"
	cred.Password = "password"
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017").SetAuth(cred)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return nil, err
	}

	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type message struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Number      int    `json:"number"`
	PublishedAt int64  `json:"published_at" bson:"published_at"`
	ConsumedAt  int64  `json:"consumed_at" bson:"consumed_at"`
}
